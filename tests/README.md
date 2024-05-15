# Test Scripts

The test script does the complete setup by building the rubix node based on the Operatng System, downloads IPFS binary for a specific version (refer `IPFS_KUBO_VERSION` variable in `run.py`) and sets up quorum and non-quorum nodes before proceeding with running all the test cases.

The test script covers the following RBT Transfer scenarios:

1. Shuttle Transfer (Success Case) <br>
  1.1 Generate 2 whole RBT for A <br>
  1.2 Transfer 0.5 from A to B <br>
  1.3 Transfer 1.499 from A to B <br>
  1.4 (Transfer 0.25 from B to A) * 4 <br>
  1.5 Transfer 1 RBT from A to B <br>
  1.6 Generate 2 whole RBT for A <br>
  1.7 Transfer 2 RBT from A to B <br>
  1.8 Transfer 0.001 from A to B <br>

2. Insufficient Balance Transfer (Failure Case) <br>
  2.1 Transferring 100 RBT from A which has zero balance <br>
  2.2 Transferring 100 RBT from B which has insufficient balance <br>

3. Transferring 0.00000009 RBT from B which is more than allowed decimal places (Failure Case)

## Prerequisites

- Python 3.10+ ([Install Ref](https://www.python.org/downloads/))
- tmux for MacOs and Ubuntu based systems ([Install Ref](https://github.com/tmux/tmux/wiki/Installing#binary-packages))
- `pip` package manger ([Install Ref](https://pip.pypa.io/en/stable/installation/))
- `requests` package. After installing Python and pip, run `pip install requests` to install this package 

## Running the tests

To start the test. Please NOTE that it must be run from the `tests` directory only.

```
python3 run.py
```

## Running tests in Docker

To run the tests in a Docker Ubuntu environment, run the following:

1. Build the image
```
docker build -t rubix_test_image_ubuntu --no-cache -f tests/docker/ubuntu_test.Dockerfile .
```

2. Run the container
```
docker run --rm --name rubix_test_container_ubuntu rubix_test_image_ubuntu
```

### Flags

The test script is equipped with CLI Parser. Following are the flags and their description

```
usage: run.py [-h] [--skip_prerequisite | --no-skip_prerequisite] [--run_nodes_only | --no-run_nodes_only] [--skip_adding_quorums | --no-skip_adding_quorums]
              [--run_tests_only | --no-run_tests_only]

CLI to run tests for Rubix

options:
  -h, --help            show this help message and exit
  --skip_prerequisite, --no-skip_prerequisite
                        skip prerequisite steps such as installing IPFS and building Rubix
  --run_nodes_only, --no-run_nodes_only
                        only run the rubix nodes and skip the setup
  --skip_adding_quorums, --no-skip_adding_quorums
                        skips adding quorums
  --run_tests_only, --no-run_tests_only
                        only proceed with running tests
```