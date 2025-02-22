use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{RwLock, Semaphore};
use tokio::time::{interval, sleep};
use lru::LruCache;

use super::{Database, Error, Result, DatabaseType, DatabaseConfig};
use super::metrics::MetricsCollector;

/// Pool statistics
#[derive(Debug, Clone)]
pub struct PoolStats {
    /// Total connections in pool
    pub total_connections: u32,
    /// Active connections
    pub active_connections: u32,
    /// Idle connections
    pub idle_connections: u32,
    /// Waiting requests
    pub waiting_requests: u32,
    /// Average wait time in milliseconds
    pub avg_wait_time: i64,
    /// Average execution time in milliseconds
    pub avg_execution_time: i64,
    /// Connection acquisition timeouts
    pub timeouts: u64,
    /// Failed health checks
    pub failed_health_checks: u64,
}

/// Cached prepared statement
#[derive(Debug)]
struct CachedStatement {
    /// SQL query
    sql: String,
    /// Last used timestamp
    last_used: Instant,
    /// Number of times used
    uses: u64,
}

/// Connection state
#[derive(Debug)]
struct PooledConnection<D: Database> {
    /// The database connection
    conn: Arc<D>,
    /// Last used timestamp
    last_used: Instant,
    /// Creation timestamp
    created_at: Instant,
    /// Number of queries executed
    queries_executed: u64,
    /// Total query execution time
    total_execution_time: Duration,
    /// Health check status
    healthy: bool,
    /// Statement cache
    statement_cache: Option<LruCache<String, CachedStatement>>,
}

impl<D: Database> PooledConnection<D> {
    /// Create a new pooled connection
    fn new(conn: D, enable_cache: bool, cache_size: usize) -> Self {
        Self {
            conn: Arc::new(conn),
            last_used: Instant::now(),
            created_at: Instant::now(),
            queries_executed: 0,
            total_execution_time: Duration::from_secs(0),
            healthy: true,
            statement_cache: if enable_cache {
                Some(LruCache::new(cache_size))
            } else {
                None
            },
        }
    }

    /// Get a cached statement or prepare a new one
    async fn get_statement(&mut self, sql: &str) -> Result<()> {
        if let Some(cache) = &mut self.statement_cache {
            if let Some(stmt) = cache.get_mut(sql) {
                stmt.last_used = Instant::now();
                stmt.uses += 1;
            } else {
                // Prepare statement
                self.conn.prepare(sql).await?;
                cache.put(sql.to_string(), CachedStatement {
                    sql: sql.to_string(),
                    last_used: Instant::now(),
                    uses: 1,
                });
            }
        }
        Ok(())
    }

    /// Record query execution
    fn record_execution(&mut self, duration: Duration) {
        self.queries_executed += 1;
        self.total_execution_time += duration;
        self.last_used = Instant::now();
    }
}

/// Pool configuration
#[derive(Debug, Clone)]
pub struct PoolConfig {
    /// Minimum number of connections
    pub min_size: u32,
    /// Maximum number of connections
    pub max_size: u32,
    /// Connection timeout
    pub connection_timeout: Duration,
    /// Health check interval
    pub health_check_interval: Duration,
    /// Connection max lifetime
    pub max_lifetime: Duration,
    /// Connection idle timeout
    pub idle_timeout: Duration,
    /// Whether to enable statement caching
    pub enable_statement_cache: bool,
    /// Maximum cached statements per connection
    pub max_cached_statements: u32,
}

impl Default for PoolConfig {
    fn default() -> Self {
        Self {
            min_size: 5,
            max_size: 20,
            connection_timeout: Duration::from_secs(30),
            health_check_interval: Duration::from_secs(30),
            max_lifetime: Duration::from_secs(3600),
            idle_timeout: Duration::from_secs(300),
            enable_statement_cache: true,
            max_cached_statements: 100,
        }
    }
}

/// Advanced connection pool with dynamic sizing and health checks
pub struct ConnectionPool<D: Database> {
    /// Pool configuration
    config: PoolConfig,
    /// Database configuration
    db_config: DatabaseConfig,
    /// Available connections
    connections: Arc<RwLock<Vec<PooledConnection<D>>>>,
    /// Connection semaphore for limiting total connections
    semaphore: Arc<Semaphore>,
    /// Metrics collector
    metrics: Arc<MetricsCollector>,
    /// Pool statistics
    stats: Arc<RwLock<PoolStats>>,
}

impl<D: Database> ConnectionPool<D> {
    /// Create a new connection pool
    pub async fn new(db_config: DatabaseConfig, pool_config: PoolConfig, metrics: Arc<MetricsCollector>) -> Result<Self> {
        let pool = Self {
            config: pool_config,
            db_config: db_config.clone(),
            connections: Arc::new(RwLock::new(Vec::new())),
            semaphore: Arc::new(Semaphore::new(pool_config.max_size as usize)),
            metrics,
            stats: Arc::new(RwLock::new(PoolStats {
                total_connections: 0,
                active_connections: 0,
                idle_connections: 0,
                waiting_requests: 0,
                avg_wait_time: 0,
                avg_execution_time: 0,
                timeouts: 0,
                failed_health_checks: 0,
            })),
        };

        // Initialize minimum connections
        pool.initialize_connections().await?;

        // Start background tasks
        pool.start_health_checks();
        pool.start_connection_reaper();
        pool.start_pool_sizing();

        Ok(pool)
    }

    /// Initialize minimum number of connections
    async fn initialize_connections(&self) -> Result<()> {
        let mut connections = self.connections.write().await;
        for _ in 0..self.config.min_size {
            let conn = self.create_connection().await?;
            connections.push(conn);
        }
        
        let mut stats = self.stats.write().await;
        stats.total_connections = self.config.min_size;
        stats.idle_connections = self.config.min_size;
        
        Ok(())
    }

    /// Create a new database connection
    async fn create_connection(&self) -> Result<PooledConnection<D>> {
        let conn = D::connect(self.db_config.clone()).await?;
        Ok(PooledConnection {
            conn: Arc::new(conn),
            last_used: Instant::now(),
            created_at: Instant::now(),
            queries_executed: 0,
            total_execution_time: Duration::from_secs(0),
            healthy: true,
            statement_cache: if self.config.enable_statement_cache {
                Some(LruCache::new(self.config.max_cached_statements as usize))
            } else {
                None
            },
        })
    }

    /// Get a connection from the pool
    pub async fn get(&self) -> Result<Arc<D>> {
        let start = Instant::now();
        let permit = match tokio::time::timeout(self.config.connection_timeout, self.semaphore.acquire()).await {
            Ok(Ok(permit)) => permit,
            Ok(Err(_)) => return Err(Error::Connection("pool closed".into())),
            Err(_) => {
                let mut stats = self.stats.write().await;
                stats.timeouts += 1;
                return Err(Error::Connection("connection timeout".into()));
            }
        };

        let mut connections = self.connections.write().await;
        let conn = if let Some(index) = connections.iter().position(|c| c.healthy) {
            // Reuse existing connection
            let conn = &mut connections[index];
            conn.last_used = Instant::now();
            conn.conn.clone()
        } else {
            // Create new connection
            let new_conn = self.create_connection().await?;
            let conn = new_conn.conn.clone();
            connections.push(new_conn);
            conn
        };

        // Update stats
        let mut stats = self.stats.write().await;
        stats.active_connections += 1;
        stats.idle_connections = stats.total_connections - stats.active_connections;
        stats.avg_wait_time = ((stats.avg_wait_time as u64 * stats.total_connections as u64) + start.elapsed().as_millis() as u64) / stats.total_connections as u64;

        permit.forget(); // Don't release the permit until connection is returned
        Ok(conn)
    }

    /// Return a connection to the pool
    pub async fn release(&self, conn: Arc<D>) {
        let mut connections = self.connections.write().await;
        if let Some(pool_conn) = connections.iter_mut().find(|c| Arc::ptr_eq(&c.conn, &conn)) {
            pool_conn.last_used = Instant::now();
            
            let mut stats = self.stats.write().await;
            stats.active_connections -= 1;
            stats.idle_connections = stats.total_connections - stats.active_connections;
        }
        self.semaphore.add_permits(1);
    }

    /// Start background health checks
    fn start_health_checks(&self) {
        let connections = self.connections.clone();
        let interval = self.config.health_check_interval;
        let stats = self.stats.clone();

        tokio::spawn(async move {
            let mut ticker = interval(interval);
            loop {
                ticker.tick().await;
                let mut conns = connections.write().await;
                let mut failed = 0;

                for conn in conns.iter_mut() {
                    match conn.conn.ping().await {
                        Ok(_) => conn.healthy = true,
                        Err(_) => {
                            conn.healthy = false;
                            failed += 1;
                        }
                    }
                }

                let mut pool_stats = stats.write().await;
                pool_stats.failed_health_checks += failed;
            }
        });
    }

    /// Start connection reaper
    fn start_connection_reaper(&self) {
        let connections = self.connections.clone();
        let config = self.config.clone();
        let stats = self.stats.clone();

        tokio::spawn(async move {
            let mut ticker = interval(Duration::from_secs(60));
            loop {
                ticker.tick().await;
                let mut conns = connections.write().await;
                let mut stats = stats.write().await;
                let now = Instant::now();

                // Remove expired and idle connections, keeping min_size
                conns.retain(|conn| {
                    let expired = now.duration_since(conn.created_at) > config.max_lifetime;
                    let idle = now.duration_since(conn.last_used) > config.idle_timeout;
                    !(expired || idle) || stats.total_connections <= config.min_size
                });

                stats.total_connections = conns.len() as u32;
                stats.idle_connections = stats.total_connections - stats.active_connections;
            }
        });
    }

    /// Start dynamic pool sizing
    fn start_pool_sizing(&self) {
        let connections = self.connections.clone();
        let config = self.config.clone();
        let stats = self.stats.clone();

        tokio::spawn(async move {
            let mut ticker = interval(Duration::from_secs(5));
            loop {
                ticker.tick().await;
                let stats = stats.read().await;
                let current_size = stats.total_connections;
                let active = stats.active_connections;
                let waiting = stats.waiting_requests;

                // Scale up if we have waiting requests and haven't hit max_size
                if waiting > 0 && current_size < config.max_size {
                    let mut conns = connections.write().await;
                    if let Ok(new_conn) = Self::create_connection().await {
                        conns.push(new_conn);
                        drop(conns);
                        let mut stats = stats.write().await;
                        stats.total_connections += 1;
                        stats.idle_connections += 1;
                    }
                }
                // Scale down if we have too many idle connections
                else if active < current_size / 2 && current_size > config.min_size {
                    let mut conns = connections.write().await;
                    if let Some(idx) = conns.iter().position(|c| !c.healthy) {
                        conns.remove(idx);
                        drop(conns);
                        let mut stats = stats.write().await;
                        stats.total_connections -= 1;
                        stats.idle_connections -= 1;
                    }
                }
            }
        });
    }

    /// Get current pool statistics
    pub async fn stats(&self) -> PoolStats {
        self.stats.read().await.clone()
    }

    /// Execute a query using a connection from the pool
    pub async fn execute(&self, sql: &str, params: &[&(dyn ToSql + Sync)]) -> Result<u64> {
        let start = Instant::now();
        let conn = self.get().await?;
        
        // Get or prepare statement
        if let Some(mut conns) = self.connections.write().await.iter_mut().find(|c| Arc::ptr_eq(&c.conn, &conn)) {
            conns.get_statement(sql).await?;
        }

        // Execute query
        let result = conn.execute(sql, params).await;

        // Record execution time
        if let Some(mut conns) = self.connections.write().await.iter_mut().find(|c| Arc::ptr_eq(&c.conn, &conn)) {
            conns.record_execution(start.elapsed());
        }

        // Update stats
        let mut stats = self.stats.write().await;
        stats.avg_execution_time = ((stats.avg_execution_time as u64 * stats.total_connections as u64) + start.elapsed().as_millis() as u64) / stats.total_connections as u64;

        // Release connection
        self.release(conn).await;

        result
    }

    /// Execute a query that returns rows using a connection from the pool
    pub async fn query(&self, sql: &str, params: &[&(dyn ToSql + Sync)]) -> Result<Vec<Row>> {
        let start = Instant::now();
        let conn = self.get().await?;
        
        // Get or prepare statement
        if let Some(mut conns) = self.connections.write().await.iter_mut().find(|c| Arc::ptr_eq(&c.conn, &conn)) {
            conns.get_statement(sql).await?;
        }

        // Execute query
        let result = conn.query(sql, params).await;

        // Record execution time
        if let Some(mut conns) = self.connections.write().await.iter_mut().find(|c| Arc::ptr_eq(&c.conn, &conn)) {
            conns.record_execution(start.elapsed());
        }

        // Update stats
        let mut stats = self.stats.write().await;
        stats.avg_execution_time = ((stats.avg_execution_time as u64 * stats.total_connections as u64) + start.elapsed().as_millis() as u64) / stats.total_connections as u64;

        // Release connection
        self.release(conn).await;

        result
    }

    /// Get statement cache statistics
    pub async fn cache_stats(&self) -> HashMap<String, u64> {
        let mut stats = HashMap::new();
        let connections = self.connections.read().await;
        
        for conn in connections.iter() {
            if let Some(cache) = &conn.statement_cache {
                for (sql, stmt) in cache.iter() {
                    *stats.entry(sql.clone()).or_default() += stmt.uses;
                }
            }
        }
        
        stats
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::time::Duration;
    use tokio::time::sleep;

    #[tokio::test]
    async fn test_pool_initialization() {
        let db_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(":memory:".into()),
            ..Default::default()
        };

        let pool_config = PoolConfig {
            min_size: 2,
            max_size: 5,
            ..Default::default()
        };

        let metrics = Arc::new(MetricsCollector::new());
        let pool = ConnectionPool::new(db_config, pool_config, metrics).await.unwrap();

        let stats = pool.stats().await;
        assert_eq!(stats.total_connections, 2);
        assert_eq!(stats.idle_connections, 2);
    }

    #[tokio::test]
    async fn test_connection_acquisition() {
        let db_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(":memory:".into()),
            ..Default::default()
        };

        let pool_config = PoolConfig {
            min_size: 1,
            max_size: 3,
            ..Default::default()
        };

        let metrics = Arc::new(MetricsCollector::new());
        let pool = ConnectionPool::new(db_config, pool_config, metrics).await.unwrap();

        // Get connections
        let conn1 = pool.get().await.unwrap();
        let conn2 = pool.get().await.unwrap();

        let stats = pool.stats().await;
        assert_eq!(stats.active_connections, 2);

        // Release connections
        pool.release(conn1).await;
        pool.release(conn2).await;

        let stats = pool.stats().await;
        assert_eq!(stats.active_connections, 0);
    }

    #[tokio::test]
    async fn test_health_checks() {
        let db_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(":memory:".into()),
            ..Default::default()
        };

        let pool_config = PoolConfig {
            min_size: 1,
            max_size: 2,
            health_check_interval: Duration::from_millis(100),
            ..Default::default()
        };

        let metrics = Arc::new(MetricsCollector::new());
        let pool = ConnectionPool::new(db_config, pool_config, metrics).await.unwrap();

        // Wait for health checks
        sleep(Duration::from_millis(200)).await;

        let stats = pool.stats().await;
        assert_eq!(stats.failed_health_checks, 0);
    }

    #[tokio::test]
    async fn test_statement_caching() {
        let db_config = DatabaseConfig {
            db_type: DatabaseType::SQLite,
            sqlite_path: Some(":memory:".into()),
            ..Default::default()
        };

        let pool_config = PoolConfig {
            min_size: 1,
            max_size: 2,
            enable_statement_cache: true,
            max_cached_statements: 10,
            ..Default::default()
        };

        let metrics = Arc::new(MetricsCollector::new());
        let pool = ConnectionPool::new(db_config, pool_config, metrics).await.unwrap();

        // Execute same query multiple times
        let sql = "SELECT 1";
        for _ in 0..5 {
            pool.query(sql, &[]).await.unwrap();
        }

        // Check cache stats
        let stats = pool.cache_stats().await;
        assert_eq!(stats.get(sql), Some(&5));
    }
} 