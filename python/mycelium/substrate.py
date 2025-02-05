"""
Python interface to Mycelium's substrate functionality.
"""

import ctypes
import json
import os
from typing import Dict, Optional

# Load the shared library
_lib_path = os.path.join(os.path.dirname(__file__), "../../build/libmycelium.so")
_lib = ctypes.CDLL(_lib_path)

# Configure function signatures
_lib.NewSubstrateClient.argtypes = [ctypes.c_char_p]
_lib.NewSubstrateClient.restype = ctypes.c_void_p

_lib.GetStake.argtypes = [ctypes.c_void_p, ctypes.c_char_p, ctypes.c_char_p]
_lib.GetStake.restype = ctypes.c_double

_lib.AddStake.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_double,  # amount
    ctypes.c_char_p,  # hotkey
    ctypes.c_char_p,  # coldkeypair
    ctypes.c_bool,    # wait_for_inclusion
    ctypes.c_bool,    # wait_for_finalization
]
_lib.AddStake.restype = ctypes.c_bool

_lib.RemoveStake.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_double,  # amount
    ctypes.c_char_p,  # hotkey
    ctypes.c_char_p,  # coldkeypair
    ctypes.c_bool,    # wait_for_inclusion
    ctypes.c_bool,    # wait_for_finalization
]
_lib.RemoveStake.restype = ctypes.c_bool

_lib.SetWeights.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_ushort,  # netuid
    ctypes.c_char_p,  # weights JSON
    ctypes.c_char_p,  # keypair
    ctypes.c_bool,    # wait_for_inclusion
    ctypes.c_bool,    # wait_for_finalization
]
_lib.SetWeights.restype = ctypes.c_bool

_lib.ServeAxon.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_ushort,  # netuid
    ctypes.c_char_p,  # ip
    ctypes.c_ushort,  # port
    ctypes.c_char_p,  # keypair
    ctypes.c_char_p,  # version
]
_lib.ServeAxon.restype = ctypes.c_bool

_lib.IsRegistered.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_char_p,  # hotkey
    ctypes.c_ushort,  # netuid
]
_lib.IsRegistered.restype = ctypes.c_bool

_lib.FreeClient.argtypes = [ctypes.c_void_p]
_lib.FreeClient.restype = None

# Add prometheus info function signatures
_lib.GetPrometheusInfo.argtypes = [
    ctypes.c_void_p,  # client
    ctypes.c_char_p,  # hotkey
    ctypes.c_ushort,  # netuid
]
_lib.GetPrometheusInfo.restype = ctypes.c_char_p

_lib.FreeString.argtypes = [ctypes.c_char_p]
_lib.FreeString.restype = None

class SubstrateError(Exception):
    """Base class for substrate-related errors."""
    pass

class SubstrateClient:
    """Client for interacting with the Mycelium substrate chain."""
    
    def __init__(self, url: str):
        """
        Initialize a new substrate client.
        
        Args:
            url: WebSocket URL of the substrate node
            
        Raises:
            SubstrateError: If client creation fails
        """
        self._client = _lib.NewSubstrateClient(url.encode())
        if not self._client:
            raise SubstrateError("Failed to create substrate client")
            
    def __del__(self):
        """Clean up the client when object is destroyed."""
        if hasattr(self, '_client') and self._client:
            _lib.FreeClient(self._client)
            self._client = None
            
    def get_stake(self, hotkey: str, coldkey: str) -> float:
        """
        Get the stake amount for a validator.
        
        Args:
            hotkey: SS58 address of the validator's hotkey
            coldkey: SS58 address of the validator's coldkey
            
        Returns:
            float: Stake amount in TAO
            
        Raises:
            SubstrateError: If stake query fails
        """
        stake = _lib.GetStake(
            self._client,
            hotkey.encode(),
            coldkey.encode()
        )
        if stake == 0:
            raise SubstrateError("Failed to get stake")
        return stake
        
    def add_stake(
        self,
        amount: float,
        hotkey: str,
        coldkeypair: str,
        wait_for_inclusion: bool = True,
        wait_for_finalization: bool = False,
    ) -> bool:
        """
        Add stake to a validator.
        
        Args:
            amount: Amount of TAO to stake
            hotkey: SS58 address of the validator's hotkey
            coldkeypair: SS58 address of the validator's coldkey keypair
            wait_for_inclusion: Wait for transaction to be included in a block
            wait_for_finalization: Wait for transaction to be finalized
            
        Returns:
            bool: True if stake was added successfully
            
        Raises:
            SubstrateError: If staking fails
        """
        success = _lib.AddStake(
            self._client,
            amount,
            hotkey.encode(),
            coldkeypair.encode(),
            wait_for_inclusion,
            wait_for_finalization
        )
        if not success:
            raise SubstrateError("Failed to add stake")
        return True
        
    def remove_stake(
        self,
        amount: float,
        hotkey: str,
        coldkeypair: str,
        wait_for_inclusion: bool = True,
        wait_for_finalization: bool = False,
    ) -> bool:
        """
        Remove stake from a validator.
        
        Args:
            amount: Amount of TAO to unstake
            hotkey: SS58 address of the validator's hotkey
            coldkeypair: SS58 address of the validator's coldkey keypair
            wait_for_inclusion: Wait for transaction to be included in a block
            wait_for_finalization: Wait for transaction to be finalized
            
        Returns:
            bool: True if stake was removed successfully
            
        Raises:
            SubstrateError: If unstaking fails
        """
        success = _lib.RemoveStake(
            self._client,
            amount,
            hotkey.encode(),
            coldkeypair.encode(),
            wait_for_inclusion,
            wait_for_finalization
        )
        if not success:
            raise SubstrateError("Failed to remove stake")
        return True
        
    def set_weights(
        self,
        netuid: int,
        weights: Dict[str, float],
        keypair: str,
        wait_for_inclusion: bool = True,
        wait_for_finalization: bool = False,
    ) -> bool:
        """
        Set weights for other validators in the network.
        
        Args:
            netuid: Network/subnet ID
            weights: Dictionary mapping validator SS58 addresses to weight values
            keypair: SS58 address of the validator's keypair
            wait_for_inclusion: Wait for transaction to be included in a block
            wait_for_finalization: Wait for transaction to be finalized
            
        Returns:
            bool: True if weights were set successfully
            
        Raises:
            SubstrateError: If weight setting fails
        """
        weights_json = json.dumps(weights).encode()
        success = _lib.SetWeights(
            self._client,
            netuid,
            weights_json,
            keypair.encode(),
            wait_for_inclusion,
            wait_for_finalization
        )
        if not success:
            raise SubstrateError("Failed to set weights")
        return True
        
    def serve_axon(
        self,
        netuid: int,
        ip: str,
        port: int,
        keypair: str,
        version: str = "1.0.0",
    ) -> bool:
        """
        Register validator's axon endpoint on the network.
        
        Args:
            netuid: Network/subnet ID
            ip: IP address to serve on
            port: Port number to serve on
            keypair: SS58 address of the validator's keypair
            version: Version string
            
        Returns:
            bool: True if registration was successful
            
        Raises:
            SubstrateError: If registration fails
        """
        success = _lib.ServeAxon(
            self._client,
            netuid,
            ip.encode(),
            port,
            keypair.encode(),
            version.encode()
        )
        if not success:
            raise SubstrateError("Failed to register axon endpoint")
        return True
        
    def is_registered(self, hotkey: str, netuid: int) -> bool:
        """
        Check if a hotkey is registered on a subnet.
        
        Args:
            hotkey: SS58 address to check
            netuid: Network/subnet ID
            
        Returns:
            bool: True if hotkey is registered
        """
        return _lib.IsRegistered(self._client, hotkey.encode(), netuid)

    def get_prometheus_info(self, hotkey: str, netuid: int) -> Optional[Dict[str, str]]:
        """
        Get prometheus metrics info for a validator.
        
        Args:
            hotkey: SS58 address of the validator
            netuid: Network/subnet ID
            
        Returns:
            Optional[Dict[str, str]]: Dictionary containing prometheus endpoint info
                                    with 'ip', 'port', and 'version' keys, or None if not found
            
        Raises:
            SubstrateError: If query fails
        """
        result = _lib.GetPrometheusInfo(
            self._client,
            hotkey.encode(),
            netuid
        )
        if not result:
            return None
            
        try:
            info = json.loads(result.decode())
            _lib.FreeString(result)
            return info
        except Exception as e:
            if result:
                _lib.FreeString(result)
            raise SubstrateError(f"Failed to parse prometheus info: {str(e)}") 