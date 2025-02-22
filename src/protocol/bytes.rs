use bytes::Bytes;
use serde::{Deserialize, Deserializer, Serialize, Serializer};
use std::ops::Deref;

/// A wrapper around bytes::Bytes that implements Serialize and Deserialize
#[derive(Debug, Clone, PartialEq)]
pub struct SerializedBytes(Bytes);

impl SerializedBytes {
    /// Create a new SerializedBytes from raw bytes
    pub fn new(bytes: impl Into<Bytes>) -> Self {
        Self(bytes.into())
    }

    /// Get the inner Bytes
    pub fn into_inner(self) -> Bytes {
        self.0
    }
}

impl Deref for SerializedBytes {
    type Target = Bytes;

    fn deref(&self) -> &Self::Target {
        &self.0
    }
}

impl From<Bytes> for SerializedBytes {
    fn from(bytes: Bytes) -> Self {
        Self(bytes)
    }
}

impl From<Vec<u8>> for SerializedBytes {
    fn from(vec: Vec<u8>) -> Self {
        Self(Bytes::from(vec))
    }
}

impl From<&[u8]> for SerializedBytes {
    fn from(slice: &[u8]) -> Self {
        Self(Bytes::copy_from_slice(slice))
    }
}

impl Serialize for SerializedBytes {
    fn serialize<S>(&self, serializer: S) -> Result<S::Ok, S::Error>
    where
        S: Serializer,
    {
        // Serialize as a byte array
        serializer.serialize_bytes(&self.0)
    }
}

impl<'de> Deserialize<'de> for SerializedBytes {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        struct BytesVisitor;

        impl<'de> serde::de::Visitor<'de> for BytesVisitor {
            type Value = SerializedBytes;

            fn expecting(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
                formatter.write_str("a byte array")
            }

            fn visit_bytes<E>(self, v: &[u8]) -> Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                Ok(SerializedBytes::from(Vec::from(v)))
            }

            fn visit_str<E>(self, v: &str) -> Result<Self::Value, E>
            where
                E: serde::de::Error,
            {
                Ok(SerializedBytes::from(Vec::from(v.as_bytes())))
            }
        }

        deserializer.deserialize_bytes(BytesVisitor)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_conversion() {
        let bytes = Bytes::from("test");
        let serialized: SerializedBytes = bytes.clone().into();
        assert_eq!(&*serialized, &bytes);

        let vec = b"test".to_vec();
        let serialized: SerializedBytes = vec.clone().into();
        assert_eq!(&*serialized, &Bytes::from(vec));

        let slice = b"test";
        let serialized: SerializedBytes = slice.as_ref().into();
        assert_eq!(&*serialized, &Bytes::from(slice.to_vec()));
    }

    #[test]
    fn test_serialization() {
        let bytes = SerializedBytes::new(b"test".to_vec());
        let serialized = rmp_serde::to_vec(&bytes).unwrap();
        let deserialized: SerializedBytes = rmp_serde::from_slice(&serialized).unwrap();
        assert_eq!(bytes, deserialized);
    }
} 