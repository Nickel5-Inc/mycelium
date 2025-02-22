use sqlparser::dialect::{GenericDialect, PostgreSqlDialect, SQLiteDialect};
use sqlparser::parser::Parser;
use sqlparser::ast::*;
use std::collections::HashSet;

use crate::database::{Error, Result, DatabaseType};

/// Represents a table operation
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct TableOperation {
    /// Table name
    pub table: String,
    /// Operation type
    pub op_type: OperationType,
    /// Affected columns
    pub columns: HashSet<String>,
    /// WHERE clause conditions
    pub conditions: Vec<String>,
}

/// Type of operation on a table
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OperationType {
    /// SELECT operation
    Select,
    /// INSERT operation
    Insert,
    /// UPDATE operation
    Update,
    /// DELETE operation
    Delete,
    /// CREATE operation
    Create,
    /// ALTER operation
    Alter,
    /// DROP operation
    Drop,
}

impl OperationType {
    /// Check if this operation conflicts with another
    pub fn conflicts_with(&self, other: &OperationType) -> bool {
        use OperationType::*;
        match (self, other) {
            // Reads don't conflict with reads
            (Select, Select) => false,
            // Writes conflict with writes
            (Insert, Insert) |
            (Update, Update) |
            (Delete, Delete) |
            (Insert, Update) |
            (Insert, Delete) |
            (Update, Delete) => true,
            // DDL conflicts with everything
            (Create, _) |
            (Alter, _) |
            (Drop, _) |
            (_, Create) |
            (_, Alter) |
            (_, Drop) => true,
            // Read-write conflicts
            (Select, _) |
            (_, Select) => false,
        }
    }
}

/// SQL statement analyzer
#[derive(Debug)]
pub struct SqlAnalyzer {
    db_type: DatabaseType,
}

impl SqlAnalyzer {
    /// Create a new SQL analyzer
    pub fn new(db_type: DatabaseType) -> Self {
        Self { db_type }
    }

    /// Parse SQL and extract table operations
    pub fn analyze(&self, sql: &str) -> Result<Vec<TableOperation>> {
        // Choose dialect based on database type
        let dialect: Box<dyn sqlparser::dialect::Dialect> = match self.db_type {
            DatabaseType::PostgreSQL => Box::new(PostgreSqlDialect {}),
            DatabaseType::SQLite => Box::new(SQLiteDialect {}),
        };

        // Parse SQL
        let ast = Parser::parse_sql(&dialect, sql)
            .map_err(|e| Error::Replication(format!("Failed to parse SQL: {}", e)))?;

        let mut operations = Vec::new();
        for stmt in ast {
            match stmt {
                Statement::Query(query) => {
                    operations.extend(self.analyze_query(*query)?);
                }
                Statement::Insert { table_name, columns, .. } => {
                    operations.push(TableOperation {
                        table: table_name.to_string(),
                        op_type: OperationType::Insert,
                        columns: columns.into_iter().map(|c| c.to_string()).collect(),
                        conditions: vec![],
                    });
                }
                Statement::Update { table_name, assignments, selection, .. } => {
                    let mut columns = HashSet::new();
                    for assignment in assignments {
                        columns.insert(assignment.id.to_string());
                    }
                    let conditions = selection.map(|expr| expr.to_string()).into_iter().collect();
                    operations.push(TableOperation {
                        table: table_name.to_string(),
                        op_type: OperationType::Update,
                        columns,
                        conditions,
                    });
                }
                Statement::Delete { table_name, selection, .. } => {
                    let conditions = selection.map(|expr| expr.to_string()).into_iter().collect();
                    operations.push(TableOperation {
                        table: table_name.to_string(),
                        op_type: OperationType::Delete,
                        columns: HashSet::new(),
                        conditions,
                    });
                }
                Statement::CreateTable { name, .. } => {
                    operations.push(TableOperation {
                        table: name.to_string(),
                        op_type: OperationType::Create,
                        columns: HashSet::new(),
                        conditions: vec![],
                    });
                }
                Statement::AlterTable { name, .. } => {
                    operations.push(TableOperation {
                        table: name.to_string(),
                        op_type: OperationType::Alter,
                        columns: HashSet::new(),
                        conditions: vec![],
                    });
                }
                Statement::Drop { object_type: ObjectType::Table, names, .. } => {
                    for name in names {
                        operations.push(TableOperation {
                            table: name.to_string(),
                            op_type: OperationType::Drop,
                            columns: HashSet::new(),
                            conditions: vec![],
                        });
                    }
                }
                _ => {}
            }
        }

        Ok(operations)
    }

    /// Analyze a SELECT query
    fn analyze_query(&self, query: Query) -> Result<Vec<TableOperation>> {
        let mut operations = Vec::new();

        if let Some(body) = query.body {
            match body {
                SetExpr::Select(select) => {
                    let mut columns = HashSet::new();
                    for item in &select.projection {
                        if let SelectItem::UnnamedExpr(expr) = item {
                            if let Expr::Identifier(ident) = expr {
                                columns.insert(ident.value.clone());
                            }
                        }
                    }

                    // Extract tables from FROM clause
                    for table_with_joins in select.from {
                        let table = table_with_joins.relation.to_string();
                        let conditions = select.selection.map(|expr| expr.to_string()).into_iter().collect();
                        operations.push(TableOperation {
                            table,
                            op_type: OperationType::Select,
                            columns: columns.clone(),
                            conditions,
                        });

                        // Handle JOINs
                        for join in table_with_joins.joins {
                            let table = join.relation.to_string();
                            operations.push(TableOperation {
                                table,
                                op_type: OperationType::Select,
                                columns: columns.clone(),
                                conditions: vec![],
                            });
                        }
                    }
                }
                _ => {}
            }
        }

        Ok(operations)
    }

    /// Check if two SQL statements conflict
    pub fn check_conflicts(&self, sql1: &str, sql2: &str) -> Result<bool> {
        let ops1 = self.analyze(sql1)?;
        let ops2 = self.analyze(sql2)?;

        // Check for conflicts between operations
        for op1 in &ops1 {
            for op2 in &ops2 {
                if op1.table == op2.table && op1.op_type.conflicts_with(&op2.op_type) {
                    // For updates, check if they affect the same rows
                    if op1.op_type == OperationType::Update && op2.op_type == OperationType::Update {
                        // If they have different WHERE clauses, they might not conflict
                        if !op1.conditions.is_empty() && !op2.conditions.is_empty() &&
                           op1.conditions != op2.conditions {
                            continue;
                        }
                    }
                    return Ok(true);
                }
            }
        }

        Ok(false)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sql_analysis() {
        let analyzer = SqlAnalyzer::new(DatabaseType::PostgreSQL);

        // Test SELECT
        let ops = analyzer.analyze("SELECT id, name FROM users WHERE age > 18").unwrap();
        assert_eq!(ops.len(), 1);
        assert_eq!(ops[0].op_type, OperationType::Select);
        assert_eq!(ops[0].table, "users");
        assert!(!ops[0].conditions.is_empty());

        // Test INSERT
        let ops = analyzer.analyze("INSERT INTO users (name, email) VALUES ('test', 'test@example.com')").unwrap();
        assert_eq!(ops.len(), 1);
        assert_eq!(ops[0].op_type, OperationType::Insert);
        assert_eq!(ops[0].table, "users");
        assert_eq!(ops[0].columns.len(), 2);

        // Test UPDATE
        let ops = analyzer.analyze("UPDATE users SET name = 'test' WHERE id = 1").unwrap();
        assert_eq!(ops.len(), 1);
        assert_eq!(ops[0].op_type, OperationType::Update);
        assert_eq!(ops[0].table, "users");
        assert_eq!(ops[0].columns.len(), 1);
        assert!(!ops[0].conditions.is_empty());

        // Test DELETE
        let ops = analyzer.analyze("DELETE FROM users WHERE id = 1").unwrap();
        assert_eq!(ops.len(), 1);
        assert_eq!(ops[0].op_type, OperationType::Delete);
        assert_eq!(ops[0].table, "users");
        assert!(!ops[0].conditions.is_empty());
    }

    #[test]
    fn test_conflict_detection() {
        let analyzer = SqlAnalyzer::new(DatabaseType::PostgreSQL);

        // Test non-conflicting operations
        assert!(!analyzer.check_conflicts(
            "SELECT * FROM users WHERE id = 1",
            "SELECT * FROM users WHERE id = 2"
        ).unwrap());

        // Test conflicting operations
        assert!(analyzer.check_conflicts(
            "UPDATE users SET name = 'Alice' WHERE id = 1",
            "UPDATE users SET name = 'Bob' WHERE id = 1"
        ).unwrap());

        // Test non-conflicting updates
        assert!(!analyzer.check_conflicts(
            "UPDATE users SET name = 'Alice' WHERE id = 1",
            "UPDATE users SET name = 'Bob' WHERE id = 2"
        ).unwrap());

        // Test DDL conflicts
        assert!(analyzer.check_conflicts(
            "ALTER TABLE users ADD COLUMN age INTEGER",
            "UPDATE users SET name = 'Bob' WHERE id = 1"
        ).unwrap());
    }
} 