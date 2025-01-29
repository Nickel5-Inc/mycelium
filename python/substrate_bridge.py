from fiber.chain import chain_utils, interface
from fiber.chain.chain_utils import query_substrate
from fiber.encrypted.validator import client as vali_client, handshake
from substrateinterface import SubstrateInterface
from substrateinterface.utils.ss58 import ss58_encode, ss58_decode
import asyncio
import json

class SubstrateBridge:
    def __init__(self, ws_url):
        self.substrate = SubstrateInterface(
            url=ws_url,
            ss58_format=42,  # Bittensor SS58 format
            type_registry_preset="substrate-node-template"
        )
        
    def verify_signature(self, hotkey, signature, message):
        try:
            # Use fiber's validator client for signature verification
            return vali_client.verify_signature(
                self.substrate,
                hotkey,
                signature,
                message
            )
        except Exception as e:
            print(f"Error verifying signature: {e}")
            return False
            
    def get_stake(self, hotkey, coldkey):
        try:
            # Query stake information using fiber's chain utils
            result = chain_utils.get_stake(
                self.substrate,
                hotkey,
                coldkey
            )
            return float(result) if result else 0.0
        except Exception as e:
            print(f"Error getting stake: {e}")
            return 0.0
            
    def serve_axon(self, hotkey, ip, port, netuid):
        try:
            # Use fiber's validator client to register axon
            success = vali_client.serve_axon(
                self.substrate,
                hotkey=hotkey,
                ip=ip,
                port=port,
                netuid=netuid
            )
            return success
        except Exception as e:
            print(f"Error serving axon: {e}")
            return False

# C-compatible wrapper functions
def init_substrate_interface(url):
    try:
        bridge = SubstrateBridge(url.decode('utf-8'))
        return bridge
    except Exception as e:
        print(f"Error initializing substrate interface: {e}")
        return None

def verify_signature(bridge, hotkey, signature, message):
    try:
        return bridge.verify_signature(
            hotkey.decode('utf-8'),
            signature.decode('utf-8'),
            message.decode('utf-8')
        )
    except Exception as e:
        print(f"Error in verify_signature wrapper: {e}")
        return False

def get_stake(bridge, hotkey, coldkey):
    try:
        return bridge.get_stake(
            hotkey.decode('utf-8'),
            coldkey.decode('utf-8')
        )
    except Exception as e:
        print(f"Error in get_stake wrapper: {e}")
        return 0.0

def serve_axon(bridge, hotkey, ip, port, netuid):
    try:
        return bridge.serve_axon(
            hotkey.decode('utf-8'),
            ip,
            port,
            netuid
        )
    except Exception as e:
        print(f"Error in serve_axon wrapper: {e}")
        return False 