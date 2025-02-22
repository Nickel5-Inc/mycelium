use async_trait::async_trait;
use tokio_postgres::{Client, Transaction as PgTransaction, Config, NoTls};
use bb8_postgres::{PostgresConnectionManager, Pool};
use std::sync::Arc;

use super::{Database, Transaction, Row, Value, ToSql, Error, Result, DatabaseConfig};

/// PostgreSQL database implementation
#[derive(Debug)]
pub struct PostgresDatabase {
    pool: Pool<PostgresConnectionManager<NoTls>>,
}

/// PostgreSQL transaction implementation
pub struct PostgresTransaction<'a> {
    tx: PgTransaction<'a>,
}

#[async_trait]
impl Transaction for PostgresTransaction<'_> {
    async fn commit(&mut self) -> Result<()> {
        self.tx.commit()
            .await
            .map_err(|e| Error::Transaction(e.to_string()))
    }

    async fn rollback(&mut self) -> Result<()> {
        self.tx.rollback()
            .await
            .map_err(|e| Error::Transaction(e.to_string()))
    }

    async fn execute(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64> {
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        let params: Vec<&(dyn tokio_postgres::types::ToSql + Sync)> = params.iter()
            .map(|v| v as &(dyn tokio_postgres::types::ToSql + Sync))
            .collect();

        self.tx.execute(query, &params[..])
            .await
            .map(|rows| rows as u64)
            .map_err(|e| Error::Query(e.to_string()))
    }

    async fn query(&mut self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>> {
        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        let params: Vec<&(dyn tokio_postgres::types::ToSql + Sync)> = params.iter()
            .map(|v| v as &(dyn tokio_postgres::types::ToSql + Sync))
            .collect();

        let rows = self.tx.query(query, &params[..])
            .await
            .map_err(|e| Error::Query(e.to_string()))?;

        let mut result = Vec::with_capacity(rows.len());
        for row in rows {
            let columns: Vec<String> = row.columns()
                .iter()
                .map(|col| col.name().to_string())
                .collect();

            let mut values = Vec::with_capacity(columns.len());
            for (i, _) in columns.iter().enumerate() {
                let value = if row.is_null(i) {
                    Value::Null
                } else {
                    match row.columns()[i].type_().name() {
                        "int2" | "int4" | "int8" => Value::Int(row.get(i)),
                        "float4" | "float8" => Value::Float(row.get(i)),
                        "text" | "varchar" | "char" => Value::Text(row.get(i)),
                        "bytea" => Value::Bytes(row.get(i)),
                        "bool" => Value::Bool(row.get(i)),
                        _ => Value::Text(row.get(i)), // Default to text
                    }
                };
                values.push(value);
            }

            result.push(Row { columns, values });
        }

        Ok(result)
    }
}

#[async_trait]
impl Database for PostgresDatabase {
    type Tx = PostgresTransaction<'static>;

    async fn connect(config: DatabaseConfig) -> Result<Self> {
        let mut pg_config = Config::new();
        pg_config.host(config.host.as_deref().unwrap_or("localhost"));
        pg_config.port(config.port.unwrap_or(5432));
        pg_config.user(config.user.as_deref().unwrap_or("postgres"));
        pg_config.password(config.password.as_deref().unwrap_or(""));
        pg_config.dbname(&config.database);

        if let Some(ssl_mode) = config.ssl_mode {
            pg_config.ssl_mode(ssl_mode.parse().map_err(|_| Error::Config("Invalid SSL mode".into()))?);
        }

        let manager = PostgresConnectionManager::new(pg_config, NoTls);
        let pool = Pool::builder()
            .max_size(config.max_connections)
            .build(manager)
            .await
            .map_err(|e| Error::Connection(e.to_string()))?;

        Ok(Self { pool })
    }

    async fn ping(&self) -> Result<()> {
        let client = self.pool.get().await
            .map_err(|e| Error::Connection(e.to_string()))?;
        
        client.execute("SELECT 1", &[])
            .await
            .map(|_| ())
            .map_err(|e| Error::Connection(e.to_string()))
    }

    async fn close(&self) -> Result<()> {
        self.pool.close().await;
        Ok(())
    }

    async fn begin_tx(&self) -> Result<Self::Tx> {
        let client = self.pool.get().await
            .map_err(|e| Error::Connection(e.to_string()))?;
        
        let tx = client.transaction()
            .await
            .map_err(|e| Error::Transaction(e.to_string()))?;
        
        Ok(PostgresTransaction { tx })
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
        let client = self.pool.get().await
            .map_err(|e| Error::Connection(e.to_string()))?;

        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        let params: Vec<&(dyn tokio_postgres::types::ToSql + Sync)> = params.iter()
            .map(|v| v as &(dyn tokio_postgres::types::ToSql + Sync))
            .collect();

        client.execute(query, &params[..])
            .await
            .map(|rows| rows as u64)
            .map_err(|e| Error::Query(e.to_string()))
    }

    async fn query(&self, query: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>> {
        let client = self.pool.get().await
            .map_err(|e| Error::Connection(e.to_string()))?;

        let params: Vec<Value> = params.iter()
            .map(|p| p.to_sql())
            .collect::<Result<_>>()?;
        
        let params: Vec<&(dyn tokio_postgres::types::ToSql + Sync)> = params.iter()
            .map(|v| v as &(dyn tokio_postgres::types::ToSql + Sync))
            .collect();

        let rows = client.query(query, &params[..])
            .await
            .map_err(|e| Error::Query(e.to_string()))?;

        let mut result = Vec::with_capacity(rows.len());
        for row in rows {
            let columns: Vec<String> = row.columns()
                .iter()
                .map(|col| col.name().to_string())
                .collect();

            let mut values = Vec::with_capacity(columns.len());
            for (i, _) in columns.iter().enumerate() {
                let value = if row.is_null(i) {
                    Value::Null
                } else {
                    match row.columns()[i].type_().name() {
                        "int2" | "int4" | "int8" => Value::Int(row.get(i)),
                        "float4" | "float8" => Value::Float(row.get(i)),
                        "text" | "varchar" | "char" => Value::Text(row.get(i)),
                        "bytea" => Value::Bytes(row.get(i)),
                        "bool" => Value::Bool(row.get(i)),
                        _ => Value::Text(row.get(i)), // Default to text
                    }
                };
                values.push(value);
            }

            result.push(Row { columns, values });
        }

        Ok(result)
    }
} 