"""
Weight normalization and conversion utilities.
"""

import numpy as np
from typing import Dict, List, Union

# Constants
U16_MAX = 65535  # Maximum value for u16 integers

def normalize_weights(weights: Dict[str, float]) -> Dict[str, float]:
    """
    Normalize weights to sum to 1.0.
    
    Args:
        weights: Dictionary mapping hotkeys to weight values
        
    Returns:
        Dict[str, float]: Normalized weights
    """
    if not weights:
        return {}
        
    total = sum(weights.values())
    if total == 0:
        return {k: 0.0 for k in weights}
        
    return {k: v / total for k, v in weights.items()}

def convert_weights_to_u16(weights: Dict[str, float]) -> Dict[str, int]:
    """
    Convert float weights to u16 integers.
    
    Args:
        weights: Dictionary mapping hotkeys to float weights
        
    Returns:
        Dict[str, int]: Weights converted to u16 integers
    """
    # First normalize weights
    normalized = normalize_weights(weights)
    
    # Convert to u16
    return {k: int(v * U16_MAX) for k, v in normalized.items()}

def convert_weights_from_u16(weights: Dict[str, int]) -> Dict[str, float]:
    """
    Convert u16 integer weights back to floats.
    
    Args:
        weights: Dictionary mapping hotkeys to u16 weights
        
    Returns:
        Dict[str, float]: Weights converted to floats
    """
    return {k: v / U16_MAX for k, v in weights.items()}

def validate_weights(weights: Dict[str, Union[float, int]]) -> bool:
    """
    Validate weight values.
    
    Args:
        weights: Dictionary mapping hotkeys to weight values
        
    Returns:
        bool: True if weights are valid
    """
    if not weights:
        return False
        
    # Check for negative values
    if any(v < 0 for v in weights.values()):
        return False
        
    # For u16 weights, check range
    if all(isinstance(v, int) for v in weights.values()):
        if any(v > U16_MAX for v in weights.values()):
            return False
            
    # For float weights, check range [0, 1]
    if all(isinstance(v, float) for v in weights.values()):
        if any(v > 1.0 for v in weights.values()):
            return False
            
    return True

def compute_weight_matrix(weights: Dict[str, Dict[str, float]], hotkeys: List[str]) -> np.ndarray:
    """
    Compute the weight matrix from validator weights.
    
    Args:
        weights: Nested dictionary mapping source -> target -> weight
        hotkeys: List of validator hotkeys in order
        
    Returns:
        np.ndarray: 2D weight matrix
    """
    n = len(hotkeys)
    matrix = np.zeros((n, n))
    
    # Create hotkey to index mapping
    hotkey_to_idx = {k: i for i, k in enumerate(hotkeys)}
    
    # Fill matrix
    for source, targets in weights.items():
        if source_idx := hotkey_to_idx.get(source):
            for target, weight in targets.items():
                if target_idx := hotkey_to_idx.get(target):
                    matrix[source_idx, target_idx] = weight
                    
    return matrix
