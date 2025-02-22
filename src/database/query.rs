use std::fmt;
use super::{Value, ToSql, Error, Result};

/// Represents a SQL operator
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Operator {
    Eq,    // =
    Ne,    // !=
    Lt,    // <
    Lte,   // <=
    Gt,    // >
    Gte,   // >=
    Like,  // LIKE
    In,    // IN
    NotIn, // NOT IN
    IsNull,    // IS NULL
    IsNotNull, // IS NOT NULL
}

impl fmt::Display for Operator {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Operator::Eq => write!(f, "="),
            Operator::Ne => write!(f, "!="),
            Operator::Lt => write!(f, "<"),
            Operator::Lte => write!(f, "<="),
            Operator::Gt => write!(f, ">"),
            Operator::Gte => write!(f, ">="),
            Operator::Like => write!(f, "LIKE"),
            Operator::In => write!(f, "IN"),
            Operator::NotIn => write!(f, "NOT IN"),
            Operator::IsNull => write!(f, "IS NULL"),
            Operator::IsNotNull => write!(f, "IS NOT NULL"),
        }
    }
}

/// Represents a WHERE condition
#[derive(Debug, Clone)]
pub struct Condition {
    column: String,
    op: Operator,
    value: Option<Value>,
}

impl Condition {
    /// Create a new condition
    pub fn new(column: impl Into<String>, op: Operator, value: impl ToSql) -> Result<Self> {
        Ok(Self {
            column: column.into(),
            op,
            value: Some(value.to_sql()?),
        })
    }

    /// Create an IS NULL condition
    pub fn is_null(column: impl Into<String>) -> Self {
        Self {
            column: column.into(),
            op: Operator::IsNull,
            value: None,
        }
    }

    /// Create an IS NOT NULL condition
    pub fn is_not_null(column: impl Into<String>) -> Self {
        Self {
            column: column.into(),
            op: Operator::IsNotNull,
            value: None,
        }
    }
}

/// Join type for SQL JOINs
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum JoinType {
    Inner,
    Left,
    Right,
    Full,
}

impl fmt::Display for JoinType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            JoinType::Inner => write!(f, "INNER JOIN"),
            JoinType::Left => write!(f, "LEFT JOIN"),
            JoinType::Right => write!(f, "RIGHT JOIN"),
            JoinType::Full => write!(f, "FULL JOIN"),
        }
    }
}

/// Represents a JOIN clause
#[derive(Debug, Clone)]
pub struct Join {
    join_type: JoinType,
    table: String,
    on: Vec<(String, String)>, // (left_col, right_col)
}

/// Order direction for ORDER BY
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum OrderDirection {
    Asc,
    Desc,
}

impl fmt::Display for OrderDirection {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            OrderDirection::Asc => write!(f, "ASC"),
            OrderDirection::Desc => write!(f, "DESC"),
        }
    }
}

/// Represents an ORDER BY clause
#[derive(Debug, Clone)]
pub struct OrderBy {
    column: String,
    direction: OrderDirection,
}

/// Query type
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum QueryType {
    Select,
    Insert,
    Update,
    Delete,
}

/// Query builder for constructing SQL queries
#[derive(Debug, Default)]
pub struct QueryBuilder {
    query_type: QueryType,
    table: Option<String>,
    columns: Vec<String>,
    values: Vec<Vec<Value>>, // For INSERT
    updates: Vec<(String, Value)>, // For UPDATE
    conditions: Vec<Condition>,
    joins: Vec<Join>,
    group_by: Vec<String>,
    having: Option<Condition>,
    order_by: Vec<OrderBy>,
    limit: Option<u64>,
    offset: Option<u64>,
    params: Vec<Value>,
}

impl QueryBuilder {
    /// Create a new SELECT query builder
    pub fn select() -> Self {
        Self {
            query_type: QueryType::Select,
            ..Default::default()
        }
    }

    /// Create a new INSERT query builder
    pub fn insert() -> Self {
        Self {
            query_type: QueryType::Insert,
            ..Default::default()
        }
    }

    /// Create a new UPDATE query builder
    pub fn update() -> Self {
        Self {
            query_type: QueryType::Update,
            ..Default::default()
        }
    }

    /// Create a new DELETE query builder
    pub fn delete() -> Self {
        Self {
            query_type: QueryType::Delete,
            ..Default::default()
        }
    }

    /// Set the table name
    pub fn table(mut self, table: impl Into<String>) -> Self {
        self.table = Some(table.into());
        self
    }

    /// Add columns to select
    pub fn columns(mut self, columns: impl IntoIterator<Item = impl Into<String>>) -> Self {
        self.columns.extend(columns.into_iter().map(Into::into));
        self
    }

    /// Add a WHERE condition
    pub fn where_clause(mut self, condition: Condition) -> Self {
        if let Some(value) = condition.value.clone() {
            self.params.push(value);
        }
        self.conditions.push(condition);
        self
    }

    /// Add a JOIN clause
    pub fn join(mut self, join: Join) -> Self {
        self.joins.push(join);
        self
    }

    /// Add GROUP BY columns
    pub fn group_by(mut self, columns: impl IntoIterator<Item = impl Into<String>>) -> Self {
        self.group_by.extend(columns.into_iter().map(Into::into));
        self
    }

    /// Add a HAVING condition
    pub fn having(mut self, condition: Condition) -> Self {
        if let Some(value) = condition.value.clone() {
            self.params.push(value);
        }
        self.having = Some(condition);
        self
    }

    /// Add ORDER BY clauses
    pub fn order_by(mut self, column: impl Into<String>, direction: OrderDirection) -> Self {
        self.order_by.push(OrderBy {
            column: column.into(),
            direction,
        });
        self
    }

    /// Set LIMIT
    pub fn limit(mut self, limit: u64) -> Self {
        self.limit = Some(limit);
        self
    }

    /// Set OFFSET
    pub fn offset(mut self, offset: u64) -> Self {
        self.offset = Some(offset);
        self
    }

    /// Add values for INSERT
    pub fn values(mut self, values: impl IntoIterator<Item = impl ToSql>) -> Result<Self> {
        let values: Result<Vec<Value>> = values
            .into_iter()
            .map(|v| v.to_sql())
            .collect();
        self.values.push(values?);
        Ok(self)
    }

    /// Add SET clause for UPDATE
    pub fn set(mut self, column: impl Into<String>, value: impl ToSql) -> Result<Self> {
        self.updates.push((column.into(), value.to_sql()?));
        Ok(self)
    }

    /// Build the query
    pub fn build(&self) -> Result<(String, Vec<Value>)> {
        match self.query_type {
            QueryType::Select => self.build_select(),
            QueryType::Insert => self.build_insert(),
            QueryType::Update => self.build_update(),
            QueryType::Delete => self.build_delete(),
        }
    }

    fn build_select(&self) -> Result<(String, Vec<Value>)> {
        let table = self.table.as_ref().ok_or_else(|| {
            Error::Query("No table specified".into())
        })?;

        let mut query = String::new();
        query.push_str("SELECT ");

        // Columns
        if self.columns.is_empty() {
            query.push('*');
        } else {
            query.push_str(&self.columns.join(", "));
        }

        // FROM clause
        query.push_str(" FROM ");
        query.push_str(table);

        // JOINs
        for join in &self.joins {
            query.push(' ');
            query.push_str(&join.join_type.to_string());
            query.push(' ');
            query.push_str(&join.table);
            query.push_str(" ON ");
            let conditions: Vec<String> = join.on
                .iter()
                .map(|(left, right)| format!("{} = {}", left, right))
                .collect();
            query.push_str(&conditions.join(" AND "));
        }

        // WHERE clause
        if !self.conditions.is_empty() {
            query.push_str(" WHERE ");
            let conditions: Vec<String> = self.conditions
                .iter()
                .map(|c| {
                    if c.value.is_some() {
                        format!("{} {} ${}", c.column, c.op, self.params.len())
                    } else {
                        format!("{} {}", c.column, c.op)
                    }
                })
                .collect();
            query.push_str(&conditions.join(" AND "));
        }

        // GROUP BY
        if !self.group_by.is_empty() {
            query.push_str(" GROUP BY ");
            query.push_str(&self.group_by.join(", "));
        }

        // HAVING
        if let Some(having) = &self.having {
            query.push_str(" HAVING ");
            if having.value.is_some() {
                query.push_str(&format!("{} {} ${}", having.column, having.op, self.params.len()));
            } else {
                query.push_str(&format!("{} {}", having.column, having.op));
            }
        }

        // ORDER BY
        if !self.order_by.is_empty() {
            query.push_str(" ORDER BY ");
            let order_by: Vec<String> = self.order_by
                .iter()
                .map(|o| format!("{} {}", o.column, o.direction))
                .collect();
            query.push_str(&order_by.join(", "));
        }

        // LIMIT
        if let Some(limit) = self.limit {
            query.push_str(&format!(" LIMIT {}", limit));
        }

        // OFFSET
        if let Some(offset) = self.offset {
            query.push_str(&format!(" OFFSET {}", offset));
        }

        Ok((query, self.params.clone()))
    }

    fn build_insert(&self) -> Result<(String, Vec<Value>)> {
        let table = self.table.as_ref().ok_or_else(|| {
            Error::Query("No table specified".into())
        })?;

        if self.columns.is_empty() {
            return Err(Error::Query("No columns specified for INSERT".into()));
        }

        if self.values.is_empty() {
            return Err(Error::Query("No values specified for INSERT".into()));
        }

        let mut query = String::new();
        query.push_str("INSERT INTO ");
        query.push_str(table);
        query.push_str(" (");
        query.push_str(&self.columns.join(", "));
        query.push_str(") VALUES ");

        let mut params = Vec::new();
        let mut value_groups = Vec::new();

        for values in &self.values {
            if values.len() != self.columns.len() {
                return Err(Error::Query("Column and value count mismatch".into()));
            }

            let param_positions: Vec<String> = (1..=values.len())
                .map(|i| format!("${}", params.len() + i))
                .collect();
            value_groups.push(format!("({})", param_positions.join(", ")));
            params.extend(values.iter().cloned());
        }

        query.push_str(&value_groups.join(", "));
        Ok((query, params))
    }

    fn build_update(&self) -> Result<(String, Vec<Value>)> {
        let table = self.table.as_ref().ok_or_else(|| {
            Error::Query("No table specified".into())
        })?;

        if self.updates.is_empty() {
            return Err(Error::Query("No SET clauses for UPDATE".into()));
        }

        let mut query = String::new();
        query.push_str("UPDATE ");
        query.push_str(table);
        query.push_str(" SET ");

        let mut params = Vec::new();
        let updates: Vec<String> = self.updates
            .iter()
            .map(|(col, val)| {
                params.push(val.clone());
                format!("{} = ${}", col, params.len())
            })
            .collect();
        query.push_str(&updates.join(", "));

        // WHERE clause
        if !self.conditions.is_empty() {
            query.push_str(" WHERE ");
            let conditions: Vec<String> = self.conditions
                .iter()
                .map(|c| {
                    if let Some(value) = &c.value {
                        params.push(value.clone());
                        format!("{} {} ${}", c.column, c.op, params.len())
                    } else {
                        format!("{} {}", c.column, c.op)
                    }
                })
                .collect();
            query.push_str(&conditions.join(" AND "));
        }

        Ok((query, params))
    }

    fn build_delete(&self) -> Result<(String, Vec<Value>)> {
        let table = self.table.as_ref().ok_or_else(|| {
            Error::Query("No table specified".into())
        })?;

        let mut query = String::new();
        query.push_str("DELETE FROM ");
        query.push_str(table);

        let mut params = Vec::new();
        
        // WHERE clause
        if !self.conditions.is_empty() {
            query.push_str(" WHERE ");
            let conditions: Vec<String> = self.conditions
                .iter()
                .map(|c| {
                    if let Some(value) = &c.value {
                        params.push(value.clone());
                        format!("{} {} ${}", c.column, c.op, params.len())
                    } else {
                        format!("{} {}", c.column, c.op)
                    }
                })
                .collect();
            query.push_str(&conditions.join(" AND "));
        }

        Ok((query, params))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::database::{create_database, DatabaseConfig, DatabaseType};
    use std::time::Duration;
    use tempfile::tempdir;

    #[test]
    fn test_simple_select() {
        let query = QueryBuilder::new()
            .table("users")
            .columns(["id", "name"])
            .build()
            .unwrap();
        
        assert_eq!(query.0, "SELECT id, name FROM users");
        assert!(query.1.is_empty());
    }

    #[test]
    fn test_where_clause() {
        let query = QueryBuilder::new()
            .table("users")
            .where_clause(Condition::new("id", Operator::Eq, 1).unwrap())
            .build()
            .unwrap();
        
        assert_eq!(query.0, "SELECT * FROM users WHERE id = $1");
        assert_eq!(query.1.len(), 1);
    }

    #[test]
    fn test_complex_query() {
        let query = QueryBuilder::new()
            .table("users")
            .columns(["id", "name", "email"])
            .where_clause(Condition::new("age", Operator::Gt, 18).unwrap())
            .join(Join {
                join_type: JoinType::Left,
                table: "orders".into(),
                on: vec![("users.id".into(), "orders.user_id".into())],
            })
            .group_by(["users.id"])
            .having(Condition::new("COUNT(*)", Operator::Gt, 5).unwrap())
            .order_by("name", OrderDirection::Asc)
            .limit(10)
            .offset(20)
            .build()
            .unwrap();

        assert_eq!(
            query.0,
            "SELECT id, name, email FROM users LEFT JOIN orders ON users.id = orders.user_id \
             WHERE age > $1 GROUP BY users.id HAVING COUNT(*) > $2 ORDER BY name ASC LIMIT 10 OFFSET 20"
        );
        assert_eq!(query.1.len(), 2);
    }

    #[test]
    fn test_insert() {
        let query = QueryBuilder::insert()
            .table("users")
            .columns(["name", "email"])
            .values(["John Doe", "john@example.com"]).unwrap()
            .values(["Jane Doe", "jane@example.com"]).unwrap()
            .build()
            .unwrap();

        assert_eq!(
            query.0,
            "INSERT INTO users (name, email) VALUES ($1, $2), ($3, $4)"
        );
        assert_eq!(query.1.len(), 4);
    }

    #[test]
    fn test_update() {
        let query = QueryBuilder::update()
            .table("users")
            .set("name", "John Smith").unwrap()
            .set("email", "john.smith@example.com").unwrap()
            .where_clause(Condition::new("id", Operator::Eq, 1).unwrap())
            .build()
            .unwrap();

        assert_eq!(
            query.0,
            "UPDATE users SET name = $1, email = $2 WHERE id = $3"
        );
        assert_eq!(query.1.len(), 3);
    }

    #[test]
    fn test_delete() {
        let query = QueryBuilder::delete()
            .table("users")
            .where_clause(Condition::new("id", Operator::Eq, 1).unwrap())
            .build()
            .unwrap();

        assert_eq!(query.0, "DELETE FROM users WHERE id = $1");
        assert_eq!(query.1.len(), 1);
    }

    #[tokio::test]
    async fn test_sqlite_integration() {
        let dir = tempdir().unwrap();
        let db_path = dir.path().join("test.db");
        
        let config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(db_path.to_str().unwrap().to_string()),
            idle_timeout: Duration::from_secs(5),
            connect_timeout: Duration::from_secs(5),
            max_connections: 1,
            ..Default::default()
        };

        let db = create_database(config).await.unwrap();

        // Create test table
        let query = QueryBuilder::select()
            .table("sqlite_master")
            .build()
            .unwrap();
        db.query(&query.0, &[]).await.unwrap();

        // Create users table
        db.execute(
            "CREATE TABLE users (
                id INTEGER PRIMARY KEY,
                name TEXT NOT NULL,
                email TEXT NOT NULL UNIQUE
            )",
            &[],
        )
        .await
        .unwrap();

        // Test INSERT
        let query = QueryBuilder::insert()
            .table("users")
            .columns(["name", "email"])
            .values(["John Doe", "john@example.com"]).unwrap()
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Test SELECT
        let query = QueryBuilder::select()
            .table("users")
            .columns(["name", "email"])
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].get::<String>("name").unwrap(), "John Doe");

        // Test UPDATE
        let query = QueryBuilder::update()
            .table("users")
            .set("email", "john.doe@example.com").unwrap()
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Verify UPDATE
        let query = QueryBuilder::select()
            .table("users")
            .columns(["email"])
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows[0].get::<String>("email").unwrap(), "john.doe@example.com");

        // Test DELETE
        let query = QueryBuilder::delete()
            .table("users")
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Verify DELETE
        let query = QueryBuilder::select()
            .table("users")
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows.len(), 0);
    }

    #[tokio::test]
    async fn test_postgres_integration() {
        let config = DatabaseConfig {
            db_type: DatabaseType::PostgreSQL,
            host: Some("localhost".to_string()),
            port: Some(5432),
            user: Some("postgres".to_string()),
            password: Some("postgres".to_string()),
            database: "postgres".to_string(),
            max_connections: 5,
            idle_timeout: Duration::from_secs(5),
            ..Default::default()
        };

        let db = create_database(config).await.unwrap();

        // Create users table
        db.execute(
            "CREATE TABLE IF NOT EXISTS users (
                id SERIAL PRIMARY KEY,
                name TEXT NOT NULL,
                email TEXT NOT NULL UNIQUE
            )",
            &[],
        )
        .await
        .unwrap();

        // Test INSERT
        let query = QueryBuilder::insert()
            .table("users")
            .columns(["name", "email"])
            .values(["John Doe", "john@example.com"]).unwrap()
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Test SELECT
        let query = QueryBuilder::select()
            .table("users")
            .columns(["name", "email"])
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].get::<String>("name").unwrap(), "John Doe");

        // Test UPDATE
        let query = QueryBuilder::update()
            .table("users")
            .set("email", "john.doe@example.com").unwrap()
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Verify UPDATE
        let query = QueryBuilder::select()
            .table("users")
            .columns(["email"])
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows[0].get::<String>("email").unwrap(), "john.doe@example.com");

        // Test DELETE
        let query = QueryBuilder::delete()
            .table("users")
            .where_clause(Condition::new("name", Operator::Eq, "John Doe").unwrap())
            .build()
            .unwrap();
        db.execute(&query.0, &query.1[..]).await.unwrap();

        // Verify DELETE
        let query = QueryBuilder::select()
            .table("users")
            .build()
            .unwrap();
        let rows = db.query(&query.0, &query.1[..]).await.unwrap();
        assert_eq!(rows.len(), 0);

        // Clean up
        db.execute("DROP TABLE users", &[]).await.unwrap();
    }
} 