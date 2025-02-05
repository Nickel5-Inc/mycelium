"""
Utilities for fetching and filtering validator nodes.
"""

from typing import Dict, List, Optional, Set, Tuple
from substrateinterface import SubstrateInterface

from . import chain_utils
from .metagraph import Metagraph

def get_active_nodes(metagraph: Metagraph) -> List[str]:
    """
    Get list of active validator nodes.
    
    Args:
        metagraph: Synced metagraph instance
        
    Returns:
        List[str]: List of active validator hotkeys
    """
    return [k for k, v in metagraph.active.items() if v]

def get_nodes_by_stake(
    metagraph: Metagraph,
    min_stake: Optional[float] = None,
    max_stake: Optional[float] = None
) -> List[str]:
    """
    Get validators filtered by stake amount.
    
    Args:
        metagraph: Synced metagraph instance
        min_stake: Minimum stake in TAO (optional)
        max_stake: Maximum stake in TAO (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, stake in metagraph.stakes.items():
        if min_stake is not None and stake < min_stake:
            continue
        if max_stake is not None and stake > max_stake:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_rank(
    metagraph: Metagraph,
    min_rank: Optional[int] = None,
    max_rank: Optional[int] = None
) -> List[str]:
    """
    Get validators filtered by rank.
    
    Args:
        metagraph: Synced metagraph instance
        min_rank: Minimum rank (optional)
        max_rank: Maximum rank (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, rank in metagraph.ranks.items():
        if min_rank is not None and rank < min_rank:
            continue
        if max_rank is not None and rank > max_rank:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_trust(
    metagraph: Metagraph,
    min_trust: Optional[float] = None,
    max_trust: Optional[float] = None
) -> List[str]:
    """
    Get validators filtered by trust score.
    
    Args:
        metagraph: Synced metagraph instance
        min_trust: Minimum trust score (optional)
        max_trust: Maximum trust score (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, trust in metagraph.trust.items():
        if min_trust is not None and trust < min_trust:
            continue
        if max_trust is not None and trust > max_trust:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_consensus(
    metagraph: Metagraph,
    min_consensus: Optional[float] = None,
    max_consensus: Optional[float] = None
) -> List[str]:
    """
    Get validators filtered by consensus score.
    
    Args:
        metagraph: Synced metagraph instance
        min_consensus: Minimum consensus score (optional)
        max_consensus: Maximum consensus score (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, consensus in metagraph.consensus.items():
        if min_consensus is not None and consensus < min_consensus:
            continue
        if max_consensus is not None and consensus > max_consensus:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_incentive(
    metagraph: Metagraph,
    min_incentive: Optional[float] = None,
    max_incentive: Optional[float] = None
) -> List[str]:
    """
    Get validators filtered by incentive score.
    
    Args:
        metagraph: Synced metagraph instance
        min_incentive: Minimum incentive score (optional)
        max_incentive: Maximum incentive score (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, incentive in metagraph.incentive.items():
        if min_incentive is not None and incentive < min_incentive:
            continue
        if max_incentive is not None and incentive > max_incentive:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_dividends(
    metagraph: Metagraph,
    min_dividends: Optional[float] = None,
    max_dividends: Optional[float] = None
) -> List[str]:
    """
    Get validators filtered by dividends score.
    
    Args:
        metagraph: Synced metagraph instance
        min_dividends: Minimum dividends score (optional)
        max_dividends: Maximum dividends score (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, dividends in metagraph.dividends.items():
        if min_dividends is not None and dividends < min_dividends:
            continue
        if max_dividends is not None and dividends > max_dividends:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_emission(
    metagraph: Metagraph,
    min_emission: Optional[int] = None,
    max_emission: Optional[int] = None
) -> List[str]:
    """
    Get validators filtered by emission amount.
    
    Args:
        metagraph: Synced metagraph instance
        min_emission: Minimum emission in RAO (optional)
        max_emission: Maximum emission in RAO (optional)
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, emission in metagraph.emission.items():
        if min_emission is not None and emission < min_emission:
            continue
        if max_emission is not None and emission > max_emission:
            continue
        nodes.append(hotkey)
    return nodes

def get_nodes_by_version(
    metagraph: Metagraph,
    version: str
) -> List[str]:
    """
    Get validators running a specific version.
    
    Args:
        metagraph: Synced metagraph instance
        version: Version string to match
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, info in metagraph.axons.items():
        if info.get("version") == version:
            nodes.append(hotkey)
    return nodes

def get_nodes_by_prometheus(
    metagraph: Metagraph,
    has_prometheus: bool = True
) -> List[str]:
    """
    Get validators with/without prometheus endpoints.
    
    Args:
        metagraph: Synced metagraph instance
        has_prometheus: Whether to get nodes with (True) or without (False) prometheus
        
    Returns:
        List[str]: List of validator hotkeys meeting criteria
    """
    nodes = []
    for hotkey, info in metagraph.axons.items():
        if bool(info.get("prometheus")) == has_prometheus:
            nodes.append(hotkey)
    return nodes

def get_nodes_by_multiple_criteria(
    metagraph: Metagraph,
    min_stake: Optional[float] = None,
    max_stake: Optional[float] = None,
    min_rank: Optional[int] = None,
    max_rank: Optional[int] = None,
    min_trust: Optional[float] = None,
    max_trust: Optional[float] = None,
    min_consensus: Optional[float] = None,
    max_consensus: Optional[float] = None,
    min_incentive: Optional[float] = None,
    max_incentive: Optional[float] = None,
    min_dividends: Optional[float] = None,
    max_dividends: Optional[float] = None,
    min_emission: Optional[int] = None,
    max_emission: Optional[int] = None,
    version: Optional[str] = None,
    has_prometheus: Optional[bool] = None,
    active_only: bool = True
) -> List[str]:
    """
    Get validators matching multiple filter criteria.
    
    Args:
        metagraph: Synced metagraph instance
        min_stake: Minimum stake in TAO (optional)
        max_stake: Maximum stake in TAO (optional)
        min_rank: Minimum rank (optional)
        max_rank: Maximum rank (optional)
        min_trust: Minimum trust score (optional)
        max_trust: Maximum trust score (optional)
        min_consensus: Minimum consensus score (optional)
        max_consensus: Maximum consensus score (optional)
        min_incentive: Minimum incentive score (optional)
        max_incentive: Maximum incentive score (optional)
        min_dividends: Minimum dividends score (optional)
        max_dividends: Maximum dividends score (optional)
        min_emission: Minimum emission in RAO (optional)
        max_emission: Maximum emission in RAO (optional)
        version: Version string to match (optional)
        has_prometheus: Whether to require prometheus endpoint (optional)
        active_only: Whether to only include active nodes
        
    Returns:
        List[str]: List of validator hotkeys meeting all criteria
    """
    # Start with all nodes or active nodes
    nodes = set(get_active_nodes(metagraph) if active_only else metagraph.hotkeys)
    
    # Apply each filter
    if min_stake is not None or max_stake is not None:
        nodes &= set(get_nodes_by_stake(metagraph, min_stake, max_stake))
        
    if min_rank is not None or max_rank is not None:
        nodes &= set(get_nodes_by_rank(metagraph, min_rank, max_rank))
        
    if min_trust is not None or max_trust is not None:
        nodes &= set(get_nodes_by_trust(metagraph, min_trust, max_trust))
        
    if min_consensus is not None or max_consensus is not None:
        nodes &= set(get_nodes_by_consensus(metagraph, min_consensus, max_consensus))
        
    if min_incentive is not None or max_incentive is not None:
        nodes &= set(get_nodes_by_incentive(metagraph, min_incentive, max_incentive))
        
    if min_dividends is not None or max_dividends is not None:
        nodes &= set(get_nodes_by_dividends(metagraph, min_dividends, max_dividends))
        
    if min_emission is not None or max_emission is not None:
        nodes &= set(get_nodes_by_emission(metagraph, min_emission, max_emission))
        
    if version is not None:
        nodes &= set(get_nodes_by_version(metagraph, version))
        
    if has_prometheus is not None:
        nodes &= set(get_nodes_by_prometheus(metagraph, has_prometheus))
        
    return sorted(list(nodes))
