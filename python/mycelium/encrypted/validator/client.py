"""
Validator client implementation for Mycelium network.
"""

import time
from typing import Dict, List, Optional, Union
import numpy as np

from ...substrate import SubstrateClient, SubstrateError

class ValidatorError(Exception):
    """Base class for validator errors."""
    pass

def set_weights(
    substrate: SubstrateClient,
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
        substrate: Connected substrate client
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
        return substrate.set_weights(
            netuid=netuid,
            weights=weights,
            keypair=keypair,
            wait_for_inclusion=wait_for_inclusion,
            wait_for_finalization=wait_for_finalization
        )
    except SubstrateError as e:
        raise ValidatorError(f"Error setting weights: {str(e)}")

def serve_axon(
    substrate: SubstrateClient,
    netuid: int,
    ip: str,
    port: int,
    keypair: str,
    version: str = "1.0.0",
) -> bool:
    """
    Register validator's axon endpoint on the network.
    
    Args:
        substrate: Connected substrate client
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
        return substrate.serve_axon(
            netuid=netuid,
            ip=ip,
            port=port,
            keypair=keypair,
            version=version
        )
    except SubstrateError as e:
        raise ValidatorError(f"Error registering axon: {str(e)}")

def get_stake(
    substrate: SubstrateClient,
    hotkey: str,
    coldkey: str
) -> float:
    """
    Get the stake amount for a validator.
    
    Args:
        substrate: Connected substrate client
        hotkey: SS58 address of the validator's hotkey
        coldkey: SS58 address of the validator's coldkey
        
    Returns:
        float: Stake amount in TAO
        
    Raises:
        ValidatorError: If stake query fails
    """
    try:
        return substrate.get_stake(hotkey, coldkey)
    except SubstrateError as e:
        raise ValidatorError(f"Error getting stake: {str(e)}")

def add_stake(
    substrate: SubstrateClient,
    amount: float,
    hotkey: str,
    coldkeypair: str,
    wait_for_inclusion: bool = True,
    wait_for_finalization: bool = False,
) -> bool:
    """
    Add stake to a validator.
    
    Args:
        substrate: Connected substrate client
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
        return substrate.add_stake(
            amount=amount,
            hotkey=hotkey,
            coldkeypair=coldkeypair,
            wait_for_inclusion=wait_for_inclusion,
            wait_for_finalization=wait_for_finalization
        )
    except SubstrateError as e:
        raise ValidatorError(f"Error adding stake: {str(e)}")

def remove_stake(
    substrate: SubstrateClient,
    amount: float,
    hotkey: str,
    coldkeypair: str,
    wait_for_inclusion: bool = True,
    wait_for_finalization: bool = False,
) -> bool:
    """
    Remove stake from a validator.
    
    Args:
        substrate: Connected substrate client
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
        return substrate.remove_stake(
            amount=amount,
            hotkey=hotkey,
            coldkeypair=coldkeypair,
            wait_for_inclusion=wait_for_inclusion,
            wait_for_finalization=wait_for_finalization
        )
    except SubstrateError as e:
        raise ValidatorError(f"Error removing stake: {str(e)}")

def is_hotkey_registered(
    substrate: SubstrateClient,
    hotkey: str,
    netuid: int
) -> bool:
    """
    Check if a hotkey is registered on a subnet.
    
    Args:
        substrate: Connected substrate client
        hotkey: SS58 address to check
        netuid: Network/subnet ID
        
    Returns:
        bool: True if hotkey is registered
    """
    try:
        return substrate.is_registered(hotkey, netuid)
    except SubstrateError as e:
        raise ValidatorError(f"Error checking registration: {str(e)}")

def get_prometheus_info(
    substrate: SubstrateClient,
    hotkey: str,
    netuid: int
) -> Optional[Dict[str, str]]:
    """
    Get prometheus metrics info for a validator.
    
    Args:
        substrate: Connected substrate client
        hotkey: SS58 address to check
        netuid: Network/subnet ID
        
    Returns:
        Optional[Dict[str, str]]: Dictionary containing prometheus endpoint info
                                with 'ip', 'port', and 'version' keys, or None if not found
        
    Raises:
        ValidatorError: If query fails
    """
    try:
        return substrate.get_prometheus_info(hotkey, netuid)
    except SubstrateError as e:
        raise ValidatorError(f"Error getting prometheus info: {str(e)}")
