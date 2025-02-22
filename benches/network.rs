use bytes::Bytes;
use criterion::{black_box, criterion_group, criterion_main, Criterion};
use mycelium::protocol::{Message, MessageType, NodeId};

fn message_codec_benchmark(c: &mut Criterion) {
    use mycelium::protocol::codec::MessageCodec;
    use bytes::BytesMut;
    use tokio_util::codec::Encoder;

    let mut group = c.benchmark_group("message_codec");
    
    // Setup
    let node_id = NodeId::new("test-node");
    let mut codec = MessageCodec::default();
    let mut buf = BytesMut::new();

    // Benchmark small messages
    group.bench_function("encode_small", |b| {
        let msg = Message::new(
            MessageType::Data,
            node_id.clone(),
            Bytes::from("small payload"),
        );
        b.iter(|| {
            buf.clear();
            codec.encode(black_box(msg.clone()), &mut buf).unwrap();
        });
    });

    // Benchmark large messages
    group.bench_function("encode_large", |b| {
        let msg = Message::new(
            MessageType::Data,
            node_id.clone(),
            Bytes::from(vec![0u8; 1024 * 1024]), // 1MB payload
        );
        b.iter(|| {
            buf.clear();
            codec.encode(black_box(msg.clone()), &mut buf).unwrap();
        });
    });

    group.finish();
}

criterion_group!(benches, message_codec_benchmark);
criterion_main!(benches); 