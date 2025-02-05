"""
Validator client implementation for Mycelium network.
"""

import time
from typing import Dict, List, Optional, Union
import numpy as np
from substrateinterface import SubstrateInterface
from substrateinterface.utils.ss58 import ss58_encode, ss58_decode

from ...chain import chain_utils

class ValidatorError(Exception):
    """Base class for validator errors."""
    pass

def set_weights(
    substrate: SubstrateInterface,
    netuid: int,
    weights: Dict[str, float],
    keypair: str,
    version: str = "1.0.0",
    wait_for_inclusion: bool = True,
    wait_for_finalization: bool = False,
) -> bool:
    """
    Set weights for other validators in the network.
    
    Args:
        substrate: Connected substrate interface
        netuid: Network/subnet ID
        weights: Dictionary mapping validator SS58 addresses to weight values
        keypair: SS58 address of the validator's keypair
        version: Version string
        wait_for_inclusion: Wait for transaction to be included in a block
        wait_for_finalization: Wait for transaction to be finalized
        
    Returns:
        bool: True if weights were set successfully
        
    Raises:
        ValidatorError: If weight setting fails
    """
    try:
        # Normalize weights to u16::MAX
        max_weight = np.iinfo(np.uint16).max
        normalized_weights = {
            k: int(v * max_weight) for k, v in weights.items()
        }
        
        # Convert to bytes for Go
        weights_bytes = {
            ss58_decode(k): v for k, v in normalized_weights.items()
        }
        
        # Call Go function through chain_utils
        success = chain_utils._lib.set_weights(
            ss58_decode(keypair),
            netuid,
            weights_bytes,
            wait_for_inclusion,
            wait_for_finalization
        )
        
        if not success:
            raise ValidatorError("Failed to set weights")
            
        return True
        
    except Exception as e:
        raise ValidatorError(f"Error setting weights: {str(e)}")

def serve_axon(
    substrate: SubstrateInterface,
    netuid: int,
    ip: str,
    port: int,
    keypair: str,
    version: str = "1.0.0",
) -> bool:
    """
    Register validator's axon endpoint on the network.
    
    Args:
        substrate: Connected substrate interface
        netuid: Network/subnet ID
        ip: IP address to serve on
        port: Port number to serve on
        keypair: SS58 address of the validator's keypair
        version: Version string
        
    Returns:
        bool: True if registration was successful
        
    Raises:
        ValidatorError: If registration fails
    """
    try:
        success = chain_utils.serve_axon(
            substrate=substrate,
            hotkey=keypair,
            ip=ip,
            port=port,
            netuid=netuid,
            version=version
        )
        
        if not success:
            raise ValidatorError("Failed to register axon endpoint")
            
        return True
        
    except Exception as e:
        raise ValidatorError(f"Error registering axon: {str(e)}")

def get_stake(
    substrate: SubstrateInterface,
    hotkey: str,
    coldkey: str
) -> float:
    """
    Get the stake amount for a validator.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address of the validator's hotkey
        coldkey: SS58 address of the validator's coldkey
        
    Returns:
        float: Stake amount in TAO
        
    Raises:
        ValidatorError: If stake query fails
    """
    try:
        return chain_utils.get_stake(substrate, hotkey, coldkey)
    except Exception as e:
        raise ValidatorError(f"Error getting stake: {str(e)}")

def add_stake(
    substrate: SubstrateInterface,
    amount: float,
    hotkey: str,
    coldkeypair: str,
    wait_for_inclusion: bool = True,
    wait_for_finalization: bool = False,
) -> bool:
    """
    Add stake to a validator.
    
    Args:
        substrate: Connected substrate interface
        amount: Amount of TAO to stake
        hotkey: SS58 address of the validator's hotkey
        coldkeypair: SS58 address of the validator's coldkey keypair
        wait_for_inclusion: Wait for transaction to be included in a block
        wait_for_finalization: Wait for transaction to be finalized
        
    Returns:
        bool: True if stake was added successfully
        
    Raises:
        ValidatorError: If staking fails
    """
    try:
        success = chain_utils._lib.add_stake(
            ss58_decode(coldkeypair),
            int(amount * 1e9),  # Convert to RAO
            ss58_decode(hotkey),
            wait_for_inclusion,
            wait_for_finalization
        )
        
        if not success:
            raise ValidatorError("Failed to add stake")
            
        return True
        
    except Exception as e:
        raise ValidatorError(f"Error adding stake: {str(e)}")

def remove_stake(
    substrate: SubstrateInterface,
    amount: float,
    hotkey: str,
    coldkeypair: str,
    wait_for_inclusion: bool = True,
    wait_for_finalization: bool = False,
) -> bool:
    """
    Remove stake from a validator.
    
    Args:
        substrate: Connected substrate interface
        amount: Amount of TAO to unstake
        hotkey: SS58 address of the validator's hotkey
        coldkeypair: SS58 address of the validator's coldkey keypair
        wait_for_inclusion: Wait for transaction to be included in a block
        wait_for_finalization: Wait for transaction to be finalized
        
    Returns:
        bool: True if stake was removed successfully
        
    Raises:
        ValidatorError: If unstaking fails
    """
    try:
        success = chain_utils._lib.remove_stake(
            ss58_decode(coldkeypair),
            int(amount * 1e9),  # Convert to RAO
            ss58_decode(hotkey),
            wait_for_inclusion,
            wait_for_finalization
        )
        
        if not success:
            raise ValidatorError("Failed to remove stake")
            
        return True
        
    except Exception as e:
        raise ValidatorError(f"Error removing stake: {str(e)}")

def is_hotkey_registered(
    substrate: SubstrateInterface,
    hotkey: str,
    netuid: int
) -> bool:
    """
    Check if a hotkey is registered on a subnet.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address to check
        netuid: Network/subnet ID
        
    Returns:
        bool: True if hotkey is registered
    """
    try:
        return chain_utils.is_hotkey_registered(substrate, hotkey, netuid)
    except Exception as e:
        raise ValidatorError(f"Error checking registration: {str(e)}")

def get_prometheus_info(
    substrate: SubstrateInterface,
    hotkey: str,
    netuid: int
) -> Optional[Dict[str, str]]:
    """
    Get prometheus endpoint information for a validator.
    
    Args:
        substrate: Connected substrate interface
        hotkey: SS58 address of the validator
        netuid: Network/subnet ID
        
    Returns:
        Optional[Dict[str, str]]: Dictionary with prometheus endpoint info or None
    """
    try:
        info = chain_utils.query_subtensor(
            substrate,
            "SubtensorModule",
            "PrometheusInfo",
            [netuid, ss58_decode(hotkey)]
        )
        if info:
            return {
                "ip": info["ip"],
                "port": str(info["port"]),
                "path": info.get("path", "/metrics")
            }
        return None
    except Exception as e:
        raise ValidatorError(f"Error getting prometheus info: {str(e)}")
