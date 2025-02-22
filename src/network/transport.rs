use std::net::SocketAddr;
use std::sync::Arc;
use tokio::net::{TcpStream, TcpListener};
use std::future::Future;
use std::pin::Pin;
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncWrite, ReadBuf};
use rustls::{ClientConfig, ServerConfig};
use tokio_rustls::{TlsConnector, TlsAcceptor, client::TlsStream as ClientTlsStream, server::TlsStream as ServerTlsStream};
use crate::error::{Error, Result};
use super::cert::CertificateManager;

/// Combined TLS stream type
#[derive(Debug)]
pub enum TlsStream {
    Client(ClientTlsStream<TcpStream>),
    Server(ServerTlsStream<TcpStream>),
}

impl AsyncRead for TlsStream {
    fn poll_read(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<std::io::Result<()>> {
        match self.get_mut() {
            TlsStream::Client(s) => Pin::new(s).poll_read(cx, buf),
            TlsStream::Server(s) => Pin::new(s).poll_read(cx, buf),
        }
    }
}

impl AsyncWrite for TlsStream {
    fn poll_write(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<std::io::Result<usize>> {
        match self.get_mut() {
            TlsStream::Client(s) => Pin::new(s).poll_write(cx, buf),
            TlsStream::Server(s) => Pin::new(s).poll_write(cx, buf),
        }
    }

    fn poll_flush(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        match self.get_mut() {
            TlsStream::Client(s) => Pin::new(s).poll_flush(cx),
            TlsStream::Server(s) => Pin::new(s).poll_flush(cx),
        }
    }

    fn poll_shutdown(self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        match self.get_mut() {
            TlsStream::Client(s) => Pin::new(s).poll_shutdown(cx),
            TlsStream::Server(s) => Pin::new(s).poll_shutdown(cx),
        }
    }
}

impl Unpin for TlsStream {}

impl TlsStream {
    pub(crate) fn from_client(stream: ClientTlsStream<TcpStream>) -> Self {
        TlsStream::Client(stream)
    }

    pub(crate) fn from_server(stream: ServerTlsStream<TcpStream>) -> Self {
        TlsStream::Server(stream)
    }
}

/// Transport trait for network connections
pub trait Transport {
    /// Stream type returned by this transport
    type Stream;

    /// Connect to a remote peer
    fn connect<'a>(&'a self, addr: SocketAddr) -> impl Future<Output = Result<Self::Stream>> + Send + 'a;

    /// Accept incoming connections
    fn accept<'a>(&'a self, listener: &'a TcpListener) -> impl Future<Output = Result<(Self::Stream, SocketAddr)>> + Send + 'a;
}

/// TCP transport implementation
#[derive(Debug, Clone, Default)]
pub struct TcpTransport;

impl Transport for TcpTransport {
    type Stream = TcpStream;

    fn connect<'a>(&'a self, addr: SocketAddr) -> impl Future<Output = Result<Self::Stream>> + Send + 'a {
        async move {
            Ok(TcpStream::connect(addr).await?)
        }
    }

    fn accept<'a>(&'a self, listener: &'a TcpListener) -> impl Future<Output = Result<(Self::Stream, SocketAddr)>> + Send + 'a {
        async move {
            Ok(listener.accept().await?)
        }
    }
}

/// TLS transport implementation
#[derive(Debug, Clone)]
pub struct TlsTransport {
    cert_manager: Arc<CertificateManager>,
}

impl TlsTransport {
    /// Create a new TLS transport with certificate manager
    pub fn new(cert_manager: CertificateManager) -> Self {
        Self {
            cert_manager: Arc::new(cert_manager),
        }
    }
}

impl Transport for TlsTransport {
    type Stream = TlsStream;

    fn connect<'a>(&'a self, addr: SocketAddr) -> impl Future<Output = Result<Self::Stream>> + Send + 'a {
        async move {
            let stream = TcpStream::connect(addr).await?;
            
            let client_config = self.cert_manager.client_config()
                .ok_or_else(|| Error::network("TLS client config not available"))?;
            
            let connector = TlsConnector::from(client_config);
            let domain = addr.ip().to_string();
            
            Ok(TlsStream::Client(
                connector.connect(domain.as_str().try_into().unwrap(), stream).await?
            ))
        }
    }

    fn accept<'a>(&'a self, listener: &'a TcpListener) -> impl Future<Output = Result<(Self::Stream, SocketAddr)>> + Send + 'a {
        async move {
            let (stream, addr) = listener.accept().await?;
            
            let server_config = self.cert_manager.server_config()
                .ok_or_else(|| Error::network("TLS server config not available"))?;
            
            let acceptor = TlsAcceptor::from(server_config);
            let tls_stream = acceptor.accept(stream).await?;
            
            Ok((TlsStream::Server(tls_stream), addr))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use super::super::cert::CertificateConfig;
    use tokio::runtime::Runtime;

    #[test]
    fn test_tls_transport() {
        let rt = Runtime::new().unwrap();
        
        rt.block_on(async {
            // Create certificate manager
            let config = CertificateConfig {
                cert_path: "tests/fixtures/test_cert.pem".to_string(),
                key_path: "tests/fixtures/test_key.pem".to_string(),
                ca_path: Some("tests/fixtures/ca_cert.pem".to_string()),
                verify_client: true,
                verify_hostname: true,
            };

            let mut cert_manager = CertificateManager::new(config);
            cert_manager.load_certificates().unwrap();

            // Create TLS transport
            let transport = TlsTransport::new(cert_manager);
            let transport_clone = transport.clone();

            // Create listener
            let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
            let addr = listener.local_addr().unwrap();

            // Test connection
            let server_handle = tokio::spawn(async move {
                transport.accept(&listener).await
            });

            // Give the server a moment to start
            tokio::time::sleep(std::time::Duration::from_millis(100)).await;

            let client_result = transport_clone.connect(addr).await;
            let server_result = server_handle.await.unwrap();

            assert!(client_result.is_ok());
            assert!(server_result.is_ok());
        });
    }
} 