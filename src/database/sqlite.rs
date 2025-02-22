use async_trait::async_trait;
use rusqlite::{Connection, Transaction as SqliteTransaction, params};
use tokio::sync::Mutex;
use std::sync::Arc;

use super::{Database, Transaction, Row, Value, ToSql, Error, Result, DatabaseConfig};

/// SQLite database implementation
#[derive(Debug)]
pub struct SQLiteDatabase {
    conn: Arc<Mutex<Connection>>,
}

/// SQLite transaction implementation
pub struct SQLiteTransaction<'a> {
    tx: SqliteTransaction<'a>,
}

#[async_trait]
impl Transaction for SQLiteTransaction<'_> {
    async fn commit(&mut self) -> Result<()> {
        self.tx.commit()
            .map_err(|e| Error::Transaction(e.to_string()))
    }

    async fn rollback(&mut self) -> Result<()> {
        self.tx.rollback()
            .map_err(|e| Error::Transaction(e.to_string()))
    }

    async fn execute(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64> {
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        self.tx.execute(query, rusqlite::params_from_iter(params.iter()))
            .map(|rows| rows as u64)
            .map_err(|e| Error::Query(e.to_string()))
    }

    async fn query(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>> {
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;

        let mut stmt = self.tx.prepare(query)
            .map_err(|e| Error::Query(e.to_string()))?;

        let column_names: Vec<String> = stmt.column_names()
            .iter()
            .map(|&name| name.to_string())
            .collect();

        let rows = stmt.query_map(rusqlite::params_from_iter(params.iter()), |row| {
            let mut values = Vec::with_capacity(column_names.len());
            for i in 0..column_names.len() {
                let value = match row.get_ref(i)? {
                    rusqlite::types::ValueRef::Null => Value::Null,
                    rusqlite::types::ValueRef::Integer(i) => Value::Int(i),
                    rusqlite::types::ValueRef::Real(f) => Value::Float(f),
                    rusqlite::types::ValueRef::Text(s) => Value::Text(s.to_string()),
                    rusqlite::types::ValueRef::Blob(b) => Value::Bytes(b.to_vec()),
                };
                values.push(value);
            }
            Ok(Row {
                columns: column_names.clone(),
                values,
            })
        })
        .map_err(|e| Error::Query(e.to_string()))?;

        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(|e| Error::Query(e.to_string()))?);
        }
        Ok(result)
    }
}

#[async_trait]
impl Database for SQLiteDatabase {
    type Tx = SQLiteTransaction<'static>;

    async fn connect(config: DatabaseConfig) -> Result<Self> {
        let path = config.sqlite_path
            .ok_or_else(|| Error::Config("SQLite path not provided".into()))?;
            
        let conn = Connection::open(path)
            .map_err(|e| Error::Connection(e.to_string()))?;

        // Enable foreign keys
        conn.execute("PRAGMA foreign_keys = ON", [])
            .map_err(|e| Error::Connection(e.to_string()))?;

        Ok(Self {
            conn: Arc::new(Mutex::new(conn)),
        })
    }

    async fn ping(&self) -> Result<()> {
        let conn = self.conn.lock().await;
        conn.execute("SELECT 1", [])
            .map(|_| ())
            .map_err(|e| Error::Connection(e.to_string()))
    }

    async fn close(&self) -> Result<()> {
        // SQLite connections are automatically closed when dropped
        Ok(())
    }

    async fn begin_tx(&self) -> Result<Self::Tx> {
        let conn = self.conn.lock().await;
        let tx = conn.transaction()
            .map_err(|e| Error::Transaction(e.to_string()))?;
        Ok(SQLiteTransaction { tx })
    }

    async fn with_tx<F, T>(&self, f: F) -> Result<T>
    where
        F: FnOnce(&mut Self::Tx) -> Result<T> + Send + 'static,
        T: Send + 'static,
    {
        let mut tx = self.begin_tx().await?;
        match f(&mut tx) {
            Ok(result) => {
                tx.commit().await?;
                Ok(result)
            }
            Err(e) => {
                tx.rollback().await?;
                Err(e)
            }
        }
    }

    async fn execute(&self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64> {
        let conn = self.conn.lock().await;
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        conn.execute(query, rusqlite::params_from_iter(params.iter()))
            .map(|rows| rows as u64)
            .map_err(|e| Error::Query(e.to_string()))
    }

    async fn query(&self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>> {
        let conn = self.conn.lock().await;
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;

        let mut stmt = conn.prepare(query)
            .map_err(|e| Error::Query(e.to_string()))?;

        let column_names: Vec<String> = stmt.column_names()
            .iter()
            .map(|&name| name.to_string())
            .collect();

        let rows = stmt.query_map(rusqlite::params_from_iter(params.iter()), |row| {
            let mut values = Vec::with_capacity(column_names.len());
            for i in 0..column_names.len() {
                let value = match row.get_ref(i)? {
                    rusqlite::types::ValueRef::Null => Value::Null,
                    rusqlite::types::ValueRef::Integer(i) => Value::Int(i),
                    rusqlite::types::ValueRef::Real(f) => Value::Float(f),
                    rusqlite::types::ValueRef::Text(s) => Value::Text(s.to_string()),
                    rusqlite::types::ValueRef::Blob(b) => Value::Bytes(b.to_vec()),
                };
                values.push(value);
            }
            Ok(Row {
                columns: column_names.clone(),
                values,
            })
        })
        .map_err(|e| Error::Query(e.to_string()))?;

        let mut result = Vec::new();
        for row in rows {
            result.push(row.map_err(|e| Error::Query(e.to_string()))?);
        }
        Ok(result)
    }
} 