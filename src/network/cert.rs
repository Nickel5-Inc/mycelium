use std::fs::File;
use std::io::BufReader;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use rustls::{Certificate, PrivateKey};
use rustls::server::{ServerConfig, AllowAnyAuthenticatedClient};
use rustls::client::ClientConfig;
use tokio_rustls::TlsAcceptor;
use x509_parser::prelude::*;

use crate::error::{Error, Result};

/// Certificate configuration options
#[derive(Debug, Clone)]
pub struct CertificateConfig {
    /// Path to certificate file (PEM format)
    pub cert_path: String,
    /// Path to private key file (PEM format)
    pub key_path: String,
    /// Optional path to CA certificate for chain validation
    pub ca_path: Option<String>,
    /// Whether to verify client certificates
    pub verify_client: bool,
    /// Whether to verify hostname in server certificates
    pub verify_hostname: bool,
}

/// Certificate manager handling TLS certificate operations
#[derive(Debug)]
pub struct CertificateManager {
    config: CertificateConfig,
    server_config: Option<Arc<ServerConfig>>,
    client_config: Option<Arc<ClientConfig>>,
}

impl CertificateManager {
    /// Create a new certificate manager
    pub fn new(config: CertificateConfig) -> Self {
        Self {
            config,
            server_config: None,
            client_config: None,
        }
    }

    /// Load and validate certificates
    pub fn load_certificates(&mut self) -> Result<()> {
        // Load certificate and private key
        let cert_chain = Self::load_cert_chain(&self.config.cert_path)?;
        let private_key = Self::load_private_key(&self.config.key_path)?;

        // Validate certificate expiry
        Self::validate_cert_expiry(&cert_chain[0])?;

        // Create server config if needed
        if self.config.verify_client {
            let server_config = ServerConfig::builder()
                .with_safe_defaults()
                .with_client_cert_verifier(Arc::new(AllowAnyAuthenticatedClient::new(Self::load_root_store()?)))
                .with_single_cert(cert_chain.clone(), private_key.clone())
                .map_err(|e| Error::network(format!("failed to set server certificate: {}", e)))?;
            
            self.server_config = Some(Arc::new(server_config));
        }

        // Create client config
        let root_store = Self::load_root_store()?;
        let mut client_config = ClientConfig::builder()
            .with_safe_defaults()
            .with_root_certificates(root_store)
            .with_client_auth_cert(cert_chain, private_key)
            .map_err(|e| Error::network(format!("failed to set client certificate: {}", e)))?;

        if !self.config.verify_hostname {
            client_config.enable_sni = false;
            // TODO: Add custom ServerCertVerifier that skips hostname verification
        }

        self.client_config = Some(Arc::new(client_config));

        Ok(())
    }

    /// Load certificate chain from PEM file
    fn load_cert_chain(path: &str) -> Result<Vec<Certificate>> {
        let file = File::open(path)
            .map_err(|e| Error::network(format!("failed to open certificate file: {}", e)))?;
        let mut reader = BufReader::new(file);
        let certs = rustls_pemfile::certs(&mut reader)
            .map_err(|e| Error::network(format!("failed to parse certificate: {}", e)))?;
        Ok(certs.into_iter().map(Certificate).collect())
    }

    /// Load private key from PEM file
    fn load_private_key(path: &str) -> Result<PrivateKey> {
        let file = File::open(path)
            .map_err(|e| Error::network(format!("failed to open private key file: {}", e)))?;
        let mut reader = BufReader::new(file);
        let key = rustls_pemfile::pkcs8_private_keys(&mut reader)
            .map_err(|e| Error::network(format!("failed to parse private key: {}", e)))?
            .first()
            .ok_or_else(|| Error::network("no private key found"))?
            .clone();
        Ok(PrivateKey(key))
    }

    /// Load root certificate store
    fn load_root_store() -> Result<rustls::RootCertStore> {
        let mut root_store = rustls::RootCertStore::empty();
        root_store.add_trust_anchors(webpki_roots::TLS_SERVER_ROOTS.iter().map(|ta| {
            rustls::OwnedTrustAnchor::from_subject_spki_name_constraints(
                ta.subject,
                ta.spki,
                ta.name_constraints,
            )
        }));
        Ok(root_store)
    }

    /// Validate certificate expiry
    fn validate_cert_expiry(cert: &Certificate) -> Result<()> {
        let (_, cert) = X509Certificate::from_der(&cert.0)
            .map_err(|e| Error::network(format!("failed to parse X509 certificate: {}", e)))?;

        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;

        if now < cert.validity().not_before.timestamp() {
            return Err(Error::network("certificate not yet valid"));
        }

        if now > cert.validity().not_after.timestamp() {
            return Err(Error::network("certificate has expired"));
        }

        Ok(())
    }

    /// Get server TLS config
    pub fn server_config(&self) -> Option<Arc<ServerConfig>> {
        self.server_config.clone()
    }

    /// Get client TLS config
    pub fn client_config(&self) -> Option<Arc<ClientConfig>> {
        self.client_config.clone()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs::write;
    use tempfile::tempdir;

    #[test]
    fn test_certificate_loading() {
        let config = CertificateConfig {
            cert_path: "tests/fixtures/test_cert.pem".to_string(),
            key_path: "tests/fixtures/test_key.pem".to_string(),
            ca_path: Some("tests/fixtures/ca_cert.pem".to_string()),
            verify_client: true,
            verify_hostname: true,
        };

        let mut cert_manager = CertificateManager::new(config);
        assert!(cert_manager.load_certificates().is_ok());
        assert!(cert_manager.server_config.is_some());
        assert!(cert_manager.client_config.is_some());
    }
} 