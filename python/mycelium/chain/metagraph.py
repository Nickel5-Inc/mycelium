"""
Metagraph implementation for managing network state.
"""

import time
from typing import Dict, List, Optional, Set, Tuple
import numpy as np
from substrateinterface import SubstrateInterface
from substrateinterface.utils.ss58 import ss58_encode, ss58_decode

from . import chain_utils

class MetagraphError(Exception):
    """Base class for metagraph errors."""
    pass

class Metagraph:
    """
    Manages and queries the state of the network.
    """
    
    def __init__(self, netuid: int, substrate: Optional[SubstrateInterface] = None):
        """
        Initialize a new metagraph instance.
        
        Args:
            netuid: Network/subnet ID
            substrate: Optional substrate interface (will be created if not provided)
        """
        self.netuid = netuid
        self.substrate = substrate
        
        # Network parameters
        self.n: int = 0  # Number of neurons
        self.block: int = 0
        
        # Validator information
        self.hotkeys: List[str] = []
        self.coldkeys: Dict[str, str] = {}  # hotkey -> coldkey
        self.stakes: Dict[str, float] = {}  # hotkey -> stake
        self.ranks: Dict[str, int] = {}  # hotkey -> rank
        self.trust: Dict[str, float] = {}  # hotkey -> trust
        self.consensus: Dict[str, float] = {}  # hotkey -> consensus
        self.incentive: Dict[str, float] = {}  # hotkey -> incentive
        self.dividends: Dict[str, float] = {}  # hotkey -> dividends
        self.emission: Dict[str, int] = {}  # hotkey -> emission
        self.active: Dict[str, bool] = {}  # hotkey -> active status
        self.last_update: Dict[str, int] = {}  # hotkey -> last update block
        
        # Weight matrix
        self.weights: Dict[str, Dict[str, float]] = {}  # source -> target -> weight
        
        # Axon information
        self.axons: Dict[str, Dict[str, any]] = {}  # hotkey -> axon info
        
        # Network statistics
        self.total_stake: float = 0.0
        self.total_emission: int = 0
        self.difficulty: int = 0
        self.tempo: int = 0  # blocks per step
        
        # Last sync time
        self.last_sync: float = 0.0
        
    def sync(self, block: Optional[int] = None) -> bool:
        """
        Sync the metagraph state from the chain.
        
        Args:
            block: Optional block number to sync to
            
        Returns:
            bool: True if sync was successful
            
        Raises:
            MetagraphError: If sync fails
        """
        try:
            # Get current block if not specified
            if block is None:
                block = chain_utils.get_current_block(self.substrate)
            self.block = block
            
            # Query network parameters
            self.n = chain_utils.query_subtensor(
                self.substrate,
                "SubtensorModule",
                "TotalIssuance",
                [self.netuid]
            )
            
            # Get validator set
            self.hotkeys = chain_utils.query_subtensor(
                self.substrate,
                "SubtensorModule",
                "Neurons",
                [self.netuid]
            )
            
            # Query validator information
            for hotkey in self.hotkeys:
                # Get stake
                if coldkey := self.coldkeys.get(hotkey):
                    self.stakes[hotkey] = chain_utils.get_stake(
                        self.substrate,
                        hotkey,
                        coldkey
                    )
                
                # Get weights
                self.weights[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Weights",
                    [self.netuid, ss58_decode(hotkey)]
                )
                
                # Get axon info
                info = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "AxonInfo",
                    [self.netuid, ss58_decode(hotkey)]
                )
                if info:
                    self.axons[hotkey] = {
                        "ip": info["ip"],
                        "port": info["port"],
                        "version": info["version"],
                        "prometheus": info.get("prometheus", None)
                    }
                    self.active[hotkey] = True
                else:
                    self.active[hotkey] = False
                    
                # Get ranks and scores
                self.ranks[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Rank",
                    [self.netuid, ss58_decode(hotkey)]
                )
                self.trust[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Trust",
                    [self.netuid, ss58_decode(hotkey)]
                )
                self.consensus[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Consensus",
                    [self.netuid, ss58_decode(hotkey)]
                )
                self.incentive[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Incentive",
                    [self.netuid, ss58_decode(hotkey)]
                )
                self.dividends[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Dividends",
                    [self.netuid, ss58_decode(hotkey)]
                )
                self.emission[hotkey] = chain_utils.query_subtensor(
                    self.substrate,
                    "SubtensorModule",
                    "Emission",
                    [self.netuid, ss58_decode(hotkey)]
                )
                
            # Update network statistics
            self.total_stake = sum(self.stakes.values())
            self.total_emission = sum(self.emission.values())
            self.difficulty = chain_utils.query_subtensor(
                self.substrate,
                "SubtensorModule",
                "Difficulty",
                [self.netuid]
            )
            self.tempo = chain_utils.query_subtensor(
                self.substrate,
                "SubtensorModule",
                "Tempo",
                [self.netuid]
            )
            
            self.last_sync = time.time()
            return True
            
        except Exception as e:
            raise MetagraphError(f"Error syncing metagraph: {str(e)}")
            
    def get_weights(self, hotkey: str) -> Dict[str, float]:
        """
        Get the weights set by a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            Dict[str, float]: Mapping of target hotkeys to weights
        """
        return self.weights.get(hotkey, {})
        
    def get_stake(self, hotkey: str) -> float:
        """
        Get the stake amount for a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            float: Stake amount in TAO
        """
        return self.stakes.get(hotkey, 0.0)
        
    def get_rank(self, hotkey: str) -> int:
        """
        Get the rank of a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            int: Validator rank
        """
        return self.ranks.get(hotkey, 0)
        
    def get_trust(self, hotkey: str) -> float:
        """
        Get the trust score of a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            float: Trust score
        """
        return self.trust.get(hotkey, 0.0)
        
    def get_consensus(self, hotkey: str) -> float:
        """
        Get the consensus score of a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            float: Consensus score
        """
        return self.consensus.get(hotkey, 0.0)
        
    def get_incentive(self, hotkey: str) -> float:
        """
        Get the incentive score of a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            float: Incentive score
        """
        return self.incentive.get(hotkey, 0.0)
        
    def get_dividends(self, hotkey: str) -> float:
        """
        Get the dividends score of a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            float: Dividends score
        """
        return self.dividends.get(hotkey, 0.0)
        
    def get_emission(self, hotkey: str) -> int:
        """
        Get the emission for a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            int: Emission amount in RAO
        """
        return self.emission.get(hotkey, 0)
        
    def is_active(self, hotkey: str) -> bool:
        """
        Check if a validator is active.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            bool: True if validator is active
        """
        return self.active.get(hotkey, False)
        
    def get_axon_info(self, hotkey: str) -> Optional[Dict[str, any]]:
        """
        Get axon endpoint information for a validator.
        
        Args:
            hotkey: SS58 address of the validator
            
        Returns:
            Optional[Dict[str, any]]: Axon information or None
        """
        return self.axons.get(hotkey)
