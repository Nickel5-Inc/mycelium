# Mycelium Python Library

A Python library for interacting with the Mycelium network. This library serves as a drop-in replacement for the Fiber library when working with Mycelium subnets.

## Installation

```bash
pip install mycelium
```

For full functionality including machine learning support:
```bash
pip install mycelium[full]
```

## Basic Usage

```python
from mycelium.chain import chain_utils
from mycelium.chain.metagraph import Metagraph
from mycelium.encrypted.validator import client as vali_client

# Initialize connection to subnet
substrate = chain_utils.get_substrate_connection("ws://localhost:9944")

# Get metagraph state
metagraph = Metagraph(netuid=1)
metagraph.sync()

# Set weights as a validator
weights = {...}  # Your weight assignments
vali_client.set_weights(substrate, netuid=1, weights=weights)

# Serve axon endpoint
vali_client.serve_axon(substrate, ip="127.0.0.1", port=8080, netuid=1)
```

## Features

- Full compatibility with Mycelium subnet operations
- Drop-in replacement for Fiber library functions
- Substrate chain interaction utilities
- Validator and miner client implementations
- Metagraph state management
- Cryptographic operations (signing, verification)

## Documentation

For detailed documentation, visit [docs.mycelium.network](https://docs.mycelium.network).

## License

This project is licensed under the MIT License - see the LICENSE file for details. 