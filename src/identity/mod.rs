use std::fmt;
use std::str::FromStr;
use rand::Rng;
use sha2::{Sha256, Digest};
use hex::{FromHex, ToHex};

use crate::error::{Error, Result};

/// Node identity
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Identity {
    /// Node ID
    id: String,
    /// Public key
    public_key: Vec<u8>,
    /// Private key
    private_key: Option<Vec<u8>>,
}

impl Identity {
    /// Create a new identity
    pub fn new() -> Self {
        let mut rng = rand::thread_rng();
        let private_key: [u8; 32] = rng.gen();
        let public_key = Self::derive_public_key(&private_key);
        let id = Self::derive_id(&public_key);

        Self {
            id,
            public_key: public_key.to_vec(),
            private_key: Some(private_key.to_vec()),
        }
    }

    /// Create a new identity from public key
    pub fn from_public_key(public_key: Vec<u8>) -> Result<Self> {
        if public_key.len() != 32 {
            return Err(Error::identity("invalid public key length"));
        }

        let id = Self::derive_id(&public_key);
        Ok(Self {
            id,
            public_key,
            private_key: None,
        })
    }

    /// Get node ID
    pub fn id(&self) -> &str {
        &self.id
    }

    /// Get public key
    pub fn public_key(&self) -> &[u8] {
        &self.public_key
    }

    /// Sign data
    pub fn sign(&self, data: &[u8]) -> Result<Vec<u8>> {
        let private_key = self.private_key.as_ref().ok_or_else(|| {
            Error::identity("private key not available")
        })?;

        // TODO: Implement actual signing logic
        let mut hasher = Sha256::new();
        hasher.update(private_key);
        hasher.update(data);
        Ok(hasher.finalize().to_vec())
    }

    /// Verify signature
    pub fn verify(&self, data: &[u8], signature: &[u8]) -> Result<bool> {
        // TODO: Implement actual signature verification
        let mut hasher = Sha256::new();
        hasher.update(&self.public_key);
        hasher.update(data);
        Ok(hasher.finalize().as_slice() == signature)
    }

    /// Derive public key from private key
    fn derive_public_key(private_key: &[u8]) -> Vec<u8> {
        // TODO: Implement actual public key derivation
        let mut hasher = Sha256::new();
        hasher.update(private_key);
        hasher.finalize().to_vec()
    }

    /// Derive node ID from public key
    fn derive_id(public_key: &[u8]) -> String {
        let mut hasher = Sha256::new();
        hasher.update(public_key);
        hasher.finalize().encode_hex()
    }
}

impl fmt::Display for Identity {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.id)
    }
}

impl FromStr for Identity {
    type Err = Error;

    fn from_str(s: &str) -> Result<Self> {
        let bytes = Vec::from_hex(s).map_err(|_| {
            Error::identity("invalid identity format")
        })?;

        Self::from_public_key(bytes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_identity() {
        // Create new identity
        let identity = Identity::new();
        assert_eq!(identity.public_key.len(), 32);
        assert!(identity.private_key.is_some());

        // Sign and verify
        let data = b"test data";
        let signature = identity.sign(data).unwrap();
        assert!(identity.verify(data, &signature).unwrap());

        // Create from public key
        let public_identity = Identity::from_public_key(identity.public_key.clone()).unwrap();
        assert_eq!(public_identity.id, identity.id);
        assert!(public_identity.private_key.is_none());

        // Parse from string
        let parsed = Identity::from_str(&identity.id).unwrap();
        assert_eq!(parsed.id, identity.id);
    }
} 