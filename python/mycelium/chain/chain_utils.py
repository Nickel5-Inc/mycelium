"""
Chain utilities for interacting with the Mycelium substrate node.
"""

import ctypes
from typing import Optional, Dict, List, Union, Any
from substrateinterface import SubstrateInterface
from substrateinterface.utils.ss58 import ss58_encode, ss58_decode

# Load the Go shared library
_lib = ctypes.CDLL("libmycelium.so")  # We'll need to build this from our Go code

class SubstrateConnectionError(Exception):
    """Raised when connection to substrate node fails."""
    pass

def get_substrate_connection(url: str, ss58_format: int = 42) -> SubstrateInterface:
    """
    Create a connection to a substrate node.
    
    Args:
        url: WebSocket URL of the substrate node
        ss58_format: SS58 address format (default: 42 for Bittensor)
        
    Returns:
        SubstrateInterface: Connected substrate interface
        
    Raises:
        SubstrateConnectionError: If connection fails
    """
    try:
        substrate = SubstrateInterface(
            url=url,
            ss58_format=ss58_format,
            type_registry_preset="substrate-node-template"
        )
        # Test connection
        substrate.get_chain_head()
        return substrate
    except Exception as e:
        raise SubstrateConnectionError(f"Failed to connect to {url}: {str(e)}")

def get_stake(substrate: SubstrateInterface, hotkey: str, coldkey: str) -> float:
    """
    Query the stake amount for a validator.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address of the validator's hotkey
        coldkey: SS58 address of the validator's coldkey
        
    Returns:
        float: Stake amount in TAO
    """
    # Convert addresses to bytes
    hotkey_bytes = ss58_decode(hotkey)
    coldkey_bytes = ss58_decode(coldkey)
    
    # Call Go function through ctypes
    _lib.get_stake.argtypes = [ctypes.c_char_p, ctypes.c_char_p]
    _lib.get_stake.restype = ctypes.c_double
    
    stake = _lib.get_stake(
        hotkey_bytes,
        coldkey_bytes
    )
    return stake

def query_subtensor(
    substrate: SubstrateInterface,
    module: str,
    function: str,
    params: Optional[List[Any]] = None
) -> Any:
    """
    Query the subtensor storage.
    
    Args:
        substrate: Connected substrate interface
        module: Storage module name
        function: Storage function name
        params: Optional parameters for the query
        
    Returns:
        Any: Query result
    """
    try:
        if params is None:
            params = []
        result = substrate.query(
            module=module,
            storage_function=function,
            params=params
        )
        return result.value
    except Exception as e:
        raise SubstrateConnectionError(f"Query failed: {str(e)}")

def verify_signature(
    substrate: SubstrateInterface,
    hotkey: str,
    signature: bytes,
    message: bytes
) -> bool:
    """
    Verify a signature from a validator.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address of the validator's hotkey
        signature: Signature bytes
        message: Original message bytes
        
    Returns:
        bool: True if signature is valid
    """
    # Call Go function through ctypes
    _lib.verify_signature.argtypes = [
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_char_p
    ]
    _lib.verify_signature.restype = ctypes.c_bool
    
    return _lib.verify_signature(
        ss58_decode(hotkey),
        signature,
        message
    )

def serve_axon(
    substrate: SubstrateInterface,
    hotkey: str,
    ip: str,
    port: int,
    netuid: int,
    version: str = "1.0.0"
) -> bool:
    """
    Register an axon endpoint on the network.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address of the validator's hotkey
        ip: IP address to serve on
        port: Port to serve on
        netuid: Network/subnet ID
        version: Version string
        
    Returns:
        bool: True if registration successful
    """
    # Call Go function through ctypes
    _lib.serve_axon.argtypes = [
        ctypes.c_char_p,
        ctypes.c_char_p,
        ctypes.c_uint16,
        ctypes.c_uint16,
        ctypes.c_char_p
    ]
    _lib.serve_axon.restype = ctypes.c_bool
    
    return _lib.serve_axon(
        ss58_decode(hotkey),
        ip.encode('utf-8'),
        port,
        netuid,
        version.encode('utf-8')
    )

# Additional utility functions
def is_hotkey_registered(
    substrate: SubstrateInterface,
    hotkey: str,
    netuid: int
) -> bool:
    """Check if a hotkey is registered on a subnet."""
    validators = query_subtensor(
        substrate,
        "SubtensorModule",
        "Neurons",
        [netuid]
    )
    return ss58_decode(hotkey) in validators

def get_current_block(substrate: SubstrateInterface) -> int:
    """Get the current block number."""
    return substrate.get_block_number()
