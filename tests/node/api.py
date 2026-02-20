import requests
from dataclasses import dataclass
from typing import List

@dataclass
class NFTTokenChainReq:
    latest: bool
    nft: str

@dataclass
class NFTDataReply:
	BlockNo: int
	BlockId: str
	NFTData: str
	NFTOwner: str
	NFTValue: float
	Epoch: int
	TransactionID: str

@dataclass
class NFTTokenChainResp:
    status: bool
    message: str
    result: str
    NFTDataReply: List[NFTDataReply]

@dataclass
class NFTTokenChainErr:
    error: str


def api_get_nft_chain(
        req: NFTTokenChainReq, 
        server_port: str
    ) -> NFTTokenChainResp | NFTTokenChainErr:
    request_url = f"http://localhost:{server_port}/api/get-nft-token-chain-data"
    
    response = requests.get(request_url, params=req.__dict__)

    if response.status_code == 200:
        import json
        response_body: SmartContractTokenChainResp = json.loads(response.text)
        return response_body
    else:
        return NFTTokenChainErr(error=response.text)

def api_create_nft(artifact_file_path, metadata_file_path, did, server_port):
    request_url = f"http://localhost:{server_port}/api/create-nft"

    files = {
        "artifact": artifact_file_path,
        "metadata": metadata_file_path
    }

    data = {
        "did": did
    }

    response = requests.post(request_url, files=files, data=data)
    if response.status_code != 200:
        raise Exception(f"unable to generate NFT, request failed")
    
    import json
    response_body = json.loads(response.text)
    
    return response_body["result"]

@dataclass
class SmartContractTokenChainReq:
    latest: bool
    token: str

@dataclass
class SCDataReply:
    BlockNo: int
    BlockId: str
    SmartContractData: str
    Epoch: int
    InitiatorSignature: str
    ExecutorDID: str
    InitiatorSignData: str

@dataclass
class SmartContractTokenChainResp:
    status: bool
    message: str
    result: str
    SCTDataReply: List[SCDataReply]

@dataclass
class SmartContractTokenChainErr:
    error: str

def api_get_smart_contract_chain(
        req: SmartContractTokenChainReq, 
        server_port: str
    ) -> SmartContractTokenChainResp | SmartContractTokenChainErr:
    request_url = f"http://localhost:{server_port}/api/get-smart-contract-token-chain-data"
    
    response = requests.post(request_url, json=req.__dict__)

    if response.status_code == 200:
        import json
        response_body: SmartContractTokenChainResp = json.loads(response.text)
        return response_body
    else:
        return SmartContractTokenChainErr(error=response.text)

@dataclass
class RBTAccountInfoReq:
    did: str

@dataclass
class AccountInfo:
    did: str
    did_type: str
    rbt_amount: str
    pledged_rbt: str
    locked_rbt: str
    pinned_rbt: str

@dataclass
class RBTAccountInfoResp:
    status: bool
    message: str
    result: str
    account_info: AccountInfo

@dataclass
class RBTAccountInfoErr:
    error: str

def api_rbt_account_info(req: RBTAccountInfoReq, server_port: str) -> RBTAccountInfoResp | RBTAccountInfoErr:
    request_url = f"http://localhost:{server_port}/api/get-smart-contract-token-chain-data"
    
    response = requests.get(request_url, params=req.__dict__)

    if response.status_code == 200:
        import json
        response_body: RBTAccountInfoResp = json.loads(response.text)
        return response_body
    else:
        return RBTAccountInfoErr(error=response.text)
    
def api_get_available_rbt_balance(did: str, server_port: str) -> float | ValueError:
    req = RBTAccountInfoReq(did=did)
    resp = api_rbt_account_info(req=req, server_port=server_port)

    if isinstance(resp, RBTAccountInfoResp):
        return float(resp.account_info.rbt_amount)
    else:
        return ValueError(resp.error)