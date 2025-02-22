use async_trait::async_trait;
use super::{Migration, Version, Result};
use crate::database::Transaction;

/// Example migration that creates a users table
pub struct CreateUsersTable;

#[async_trait]
impl Migration for CreateUsersTable {
    fn version(&self) -> Version {
        Version(1)
    }

    fn name(&self) -> &str {
        "create_users_table"
    }

    async fn up(&self, tx: &mut dyn Transaction) -> Result<()> {
        tx.execute(
            "CREATE TABLE users (
                id INTEGER PRIMARY KEY,
                username TEXT NOT NULL UNIQUE,
                email TEXT NOT NULL UNIQUE,
                password_hash TEXT NOT NULL,
                created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
                updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
            )",
            &[],
        )
        .await?;

        // Create index on username
        tx.execute(
            "CREATE INDEX idx_users_username ON users (username)",
            &[],
        )
        .await?;

        Ok(())
    }

    async fn down(&self, tx: &mut dyn Transaction) -> Result<()> {
        tx.execute("DROP TABLE IF EXISTS users", &[]).await?;
        Ok(())
    }
} 