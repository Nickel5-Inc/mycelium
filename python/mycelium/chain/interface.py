"""
Low-level substrate interface for Mycelium network.
"""

import time
from typing import Any, Dict, List, Optional, Tuple, Union
from substrateinterface import SubstrateInterface
from substrateinterface.exceptions import SubstrateRequestException
from substrateinterface.utils.ss58 import ss58_encode, ss58_decode
from substrateinterface.base import QueryMapResult, StorageKey

class SubstrateError(Exception):
    """Base class for substrate interface errors."""
    pass

class SubstrateInterface:
    """
    Low-level interface to the substrate node.
    """
    
    def __init__(self, url: str, ss58_format: int = 42):
        """
        Initialize substrate interface.
        
        Args:
            url: WebSocket URL of the substrate node
            ss58_format: SS58 address format (default: 42 for Bittensor)
        """
        try:
            self.substrate = SubstrateInterface(
                url=url,
                ss58_format=ss58_format,
                type_registry_preset="substrate-node-template"
            )
            # Test connection
            self.substrate.get_chain_head()
        except Exception as e:
            raise SubstrateError(f"Failed to connect to {url}: {str(e)}")
            
    def query_map(
        self,
        module: str,
        storage_function: str,
        params: Optional[List[Any]] = None,
        block_hash: Optional[str] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> QueryMapResult:
        """
        Query a storage map.
        
        Args:
            module: Storage module name
            storage_function: Storage function name
            params: Optional parameters for the query
            block_hash: Optional block hash to query at
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            QueryMapResult: Query result
            
        Raises:
            SubstrateError: If query fails
        """
        retries = 0
        while True:
            try:
                return self.substrate.query_map(
                    module=module,
                    storage_function=storage_function,
                    params=params,
                    block_hash=block_hash
                )
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Query map failed: {str(e)}")
                time.sleep(retry_delay)
                
    def query(
        self,
        module: str,
        storage_function: str,
        params: Optional[List[Any]] = None,
        block_hash: Optional[str] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> Any:
        """
        Query a storage item.
        
        Args:
            module: Storage module name
            storage_function: Storage function name
            params: Optional parameters for the query
            block_hash: Optional block hash to query at
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            Any: Query result
            
        Raises:
            SubstrateError: If query fails
        """
        retries = 0
        while True:
            try:
                result = self.substrate.query(
                    module=module,
                    storage_function=storage_function,
                    params=params,
                    block_hash=block_hash
                )
                return result.value
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Query failed: {str(e)}")
                time.sleep(retry_delay)
                
    def submit_extrinsic(
        self,
        call_module: str,
        call_function: str,
        params: Dict[str, Any],
        keypair: Any,
        wait_for_inclusion: bool = True,
        wait_for_finalization: bool = False,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> str:
        """
        Submit an extrinsic to the chain.
        
        Args:
            call_module: Module containing the call
            call_function: Function to call
            params: Parameters for the call
            keypair: Keypair to sign with
            wait_for_inclusion: Wait for inclusion in block
            wait_for_finalization: Wait for finalization
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            str: Extrinsic hash
            
        Raises:
            SubstrateError: If submission fails
        """
        retries = 0
        while True:
            try:
                call = self.substrate.compose_call(
                    call_module=call_module,
                    call_function=call_function,
                    call_params=params
                )
                
                extrinsic = self.substrate.create_signed_extrinsic(
                    call=call,
                    keypair=keypair
                )
                
                response = self.substrate.submit_extrinsic(
                    extrinsic=extrinsic,
                    wait_for_inclusion=wait_for_inclusion,
                    wait_for_finalization=wait_for_finalization
                )
                
                return response.extrinsic_hash
                
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Extrinsic submission failed: {str(e)}")
                time.sleep(retry_delay)
                
    def get_block(
        self,
        block_hash: Optional[str] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> Dict[str, Any]:
        """
        Get block information.
        
        Args:
            block_hash: Optional block hash (latest if None)
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            Dict[str, Any]: Block information
            
        Raises:
            SubstrateError: If query fails
        """
        retries = 0
        while True:
            try:
                if block_hash:
                    return self.substrate.get_block(block_hash=block_hash)
                return self.substrate.get_block_header()
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Get block failed: {str(e)}")
                time.sleep(retry_delay)
                
    def get_block_number(
        self,
        block_hash: Optional[str] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> int:
        """
        Get block number.
        
        Args:
            block_hash: Optional block hash (latest if None)
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            int: Block number
            
        Raises:
            SubstrateError: If query fails
        """
        block = self.get_block(block_hash, max_retries, retry_delay)
        return block['header']['number']
        
    def get_account_info(
        self,
        address: str,
        block_hash: Optional[str] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> Dict[str, Any]:
        """
        Get account information.
        
        Args:
            address: SS58 address
            block_hash: Optional block hash (latest if None)
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Returns:
            Dict[str, Any]: Account information
            
        Raises:
            SubstrateError: If query fails
        """
        retries = 0
        while True:
            try:
                return self.substrate.query(
                    module='System',
                    storage_function='Account',
                    params=[address],
                    block_hash=block_hash
                ).value
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Get account info failed: {str(e)}")
                time.sleep(retry_delay)
                
    def get_storage_key(
        self,
        module: str,
        storage_function: str,
        params: Optional[List[Any]] = None
    ) -> StorageKey:
        """
        Create a storage key.
        
        Args:
            module: Storage module name
            storage_function: Storage function name
            params: Optional parameters for the key
            
        Returns:
            StorageKey: Storage key
            
        Raises:
            SubstrateError: If key creation fails
        """
        try:
            return self.substrate.create_storage_key(
                module=module,
                storage_function=storage_function,
                params=params
            )
        except Exception as e:
            raise SubstrateError(f"Create storage key failed: {str(e)}")
            
    def subscribe_storage_changes(
        self,
        keys: List[StorageKey],
        callback: callable,
        max_retries: int = 3,
        retry_delay: float = 1.0
    ) -> None:
        """
        Subscribe to storage changes.
        
        Args:
            keys: List of storage keys to subscribe to
            callback: Callback function for changes
            max_retries: Maximum number of retry attempts
            retry_delay: Delay between retries in seconds
            
        Raises:
            SubstrateError: If subscription fails
        """
        retries = 0
        while True:
            try:
                self.substrate.subscribe_storage(
                    keys=keys,
                    subscription_handler=callback
                )
                return
            except Exception as e:
                retries += 1
                if retries >= max_retries:
                    raise SubstrateError(f"Subscribe storage failed: {str(e)}")
                time.sleep(retry_delay)
                
    def close(self) -> None:
        """Close the substrate connection."""
        try:
            self.substrate.close()
        except:
            pass
