# Test Scripts

The test script does the complete setup by building the rubix node based on the Operatng System, downloads IPFS binary for a specific version (refer `IPFS_KUBO_VERSION` variable in `run.py`) and sets up quorum and non-quorum nodes before proceeding with running all the test cases.

## Prerequisites

- Python 3.10+ ([Install Ref](https://www.python.org/downloads/))
- tmux for MacOs and Ubuntu based systems ([Install Ref](https://github.com/tmux/tmux/wiki/Installing#binary-packages))
- `pip` package manger ([Install Ref](https://pip.pypa.io/en/stable/installation/))
- `requests` package. After installing Python and pip, run `pip install requests` to install this package 

## Running the tests

The tests are divided into 4 scripts, each representing an asset:

```
# RBT
python3 asset_rbt_tests.py

# FT
python3 asset_ft_tests.py

# NFT
python3 asset_nft_tests.py

# Smart Contract
python3 asset_smart_contract_tests.py
```
