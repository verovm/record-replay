# Ethereum Substate Recorder/Replayer
Ethereum substate recorder/replayer based on the paper:

**Yeonsoo Kim, Seongho Jeong, Kamil Jezek, Bernd Burgstaller, and Bernhard Scholz**: _An Off-The-Chain Execution Environment for Scalable Testing and Profiling of Smart Contracts_,  USENIX ATC'21

To build our recorder (`geth record-substate`) and replayer (`substate-cli`), run `make record-replay`.
Running `make rr` or `make` works exactly same as `make record-replay`.

You can find all executables including `geth` and our `substate-cli` in `build/bin/` directory.
Running with `--version` from record-replay will print both the Geth version and the record-replay version.
For example, `./geth --version` will print `geth version 1.13.15-rr0.5.0-commit` which indicates `rr0.5.0` is based on Geth `v1.13.15`.

Check [CHANGELOG.md](./CHANGELOG.md) for release notes.



## How to record transaction substates
Here is a simple way how to record substates.
1. Sync `geth` up to the block that you want to record and replay.
Geth full/snap sync will download blocks from the genesis block.
To sync blocks after the PoS update, you need to simultaneously run a consensus layer client.
Visit [Nodes and Clients](https://ethereum.org/en/developers/docs/nodes-and-clients/) page for more details.
2. Export blocks using `geth export` command.
For example, `geth export ethereum.blockchain` to export from the genesis block to the latest synced block.
3. Import the exported blocks to record substates using `geth record-substate` command from scratch.
For example, `geth record-substate ethereum.blockchain` to import and record the exported blocks from the unmodified Geth.

If you want to record a specific range of blocks `(X)-(Y)`, you need the Geth database specified by `--datadir` whose head block is `(X-1)`, and blocks `(X)-(Y)` exported to a file.
If you don't have the Geth database at block `(X-1)`, then you need to export blocks up to `(X-1)`, and import it from scratch again.

The output directory specified by `--substatedir` (default: `substate.ethereum`) is the substate DB.
The directory is a single LevelDB instance, so you must read or write the substate DB with `github.com/syndtr/goleveldb` module.
The substate DB may be corrupted if you directly write or modify any files in the directory.
Do not use LevelDB library for other languages (C++, Python, etc.) because they are incompatible with the goleveldb module.

Our recorder requires more memory to test faithful replay while writing substates to substate DB.
Therefore, it is recommended to have 32GB RAM for recording.
If you want to run without testing faithful replay, use `--skip-check-replay` option.

Our `geth record-substate` command is based on `geth import` full sync. You may want to try different options of `geth import` such as `--snapshot`, `--db.engine`, and `--state.scheme` to improve full sync speed and size. The `--datadir` path after `geth record-substate` will be at the last block of the imported chain same as `geth import`. 

For example, if you want to record from block `2_000_001` to `3_000_000`:
```bash
# Ctrl+C to stop syncing when geth finished sync blocks to record
./geth --datadir datadir-1

# Export blockchain files 0-2M, 2-3M
./geth export --datadir datadir-1 0-2M.blockchain 1 2000000
./geth export --datadir datadir-1 2-3M.blockchain 2000001 3000000

# Import blockchain files up to 2M
./geth import --datadir datadir-2 0-2M.blockchain

# Record substates from 2M to 3M
./geth record-substate --datadir datadir-2 2-3M.blockchain
```



## Substate DB

Since `rr0.2`, a substate database contains two types of key-value pairs in one [goleveldb](https://github.com/syndtr/goleveldb) instance.
The first 2 bytes of a key in a substate DB represent different data types as follows:
1. `1s`: Substate, a key is `"1s"+N+T` with transaction index `T` at block `N`.
`T` and `N` are encoded in a big-endian 64-bit binary.
2. `1c`: EVM bytecode, a key is `"1c"+codeHash` where `codeHash` is Keccak256 hash of the bytecode.

A goleveldb instance is the path of the directory that contains `*.ldb` files.
Copying or overwriting `*.ldb` does not merge two instances but corrupts the written one.
The goleveldb module and the official C++ LevelDB implementation are not compatible with each other.
Therefore, you need to write a Go program to properly read and write goleveldb instances.



## How to replay transaction with substates
`substate-cli replay` executes transaction substates in a given block range.
If `substate-cli replay` finds an execution result that is not equivalent to the recorded result,
it returns an error immediately.

For example, if you want to replay transactions from block 1,000,001 to block 2,000,000 in `substate.ethereum`:
```bash
./substate-cli replay --block-segment 1000001-2000000
```
In `--block-segment`, you can use `_` as a digit separator in block segment like `1_000_001-2_000_000`.
You can use SI unit suffix `k` and `M` to `--block-segment` for shorter notations like `1_000-2_000k` or `1-2M`.
```bash
./substate-cli replay --block-segment 1-2M
```
NOTE: When use `k` or `M` to `--block-segment`, the last digit of the first block number is `1`, not `0`, as described in the above examples.

Here are command line options for `substate-cli replay`:
```
NAME:
   substate-cli replay - replay transactions and check output consistency

USAGE:
   substate-cli replay [command options] [arguments...]

CATEGORY:
   replay

DESCRIPTION:
   
   substate-cli replay executes transactions in the given block segment
   and check output consistency for faithful replaying.

OPTIONS:
   
    --block-segment value         
          Single block segment (e.g. 1001, 1_001, 1_001-2_000, 1-2k, 1-2M)
    --skip-call-txs                (default: false)
          Skip executing CALL transactions to accounts with contract bytecode
    --skip-create-txs              (default: false)
          Skip executing CREATE transactions
    --skip-transfer-txs            (default: false)
          Skip executing transactions that only transfer ETH
    --substatedir value, --substate-db value (default: "substate.ethereum")
          Data directory for substate recorder/replayer
    --workers value                (default: 4)
          Number of worker threads (goroutines), 0 for current CPU physical cores
```

For example, if you want 32 workers to replay transactions except CREATE transactions:
```bash
./substate-cli replay --block-segment 1-2M --workers 32 --skip-create-txs
```

If you want to replay only CALL transactions and skip the other types of transactions:
```bash
./substate-cli replay --block-segment 1-2M --skip-transfer-txs --skip-create-txs
```

If you want to use a substate DB other than `substate.ethereum` (e.g. `/path/to/substate_db`):
```bash
./substate-cli replay --block-segment 1-2M --substatedir /path/to/substate_db
```

### Hard-fork assessment
To assess hard-forks with prior transactions, use `substate-cli replay-fork` command. Run `./substate-cli replay-fork --help` for more details:

```
NAME:
   substate-cli replay-fork - replay transactions with the given hard fork and compare results

USAGE:
   substate-cli replay-fork [command options] [arguments...]

CATEGORY:
   replay

DESCRIPTION:
   
   substate-cli replay executes transactions in the given block segment
   with the given hard fork config and report output comparison results.

OPTIONS:
   
    --block-segment value         
          Single block segment (e.g. 1001, 1_001, 1_001-2_000, 1-2k, 1-2M)
    --hard-fork value              (default: 17034870)
          Hard-fork block number, won't change block number in BlockEnv for NUMBER
          instruction, 1: Frontier, 1150000: Homestead, 2463000: Tangerine Whistle,
          2675000: Spurious Dragon, 4370000: Byzantium, 7280000: Constantinople +
          Petersburg, 9069000: Istanbul, 12244000: Berlin, 12965000: London, 15537394:
          Paris (The Merge), 17034870: Shanghai
    --skip-call-txs                (default: false)
          Skip executing CALL transactions to accounts with contract bytecode
    --skip-create-txs              (default: false)
          Skip executing CREATE transactions
    --skip-transfer-txs            (default: false)
          Skip executing transactions that only transfer ETH
    --substatedir value, --substate-db value (default: "substate.ethereum")
          Data directory for substate recorder/replayer
    --workers value                (default: 4)
          Number of worker threads (goroutines), 0 for current CPU physical cores
```



## Substate DB manipulation
`substate-cli db-*` commands are additional commands to directly manipulate substate DBs.

### `db-rr0.3-to-rr0.4`
`substate-cli db-rr0.3-to-rr0.4` (aliased to `substate-cli db-rlp2proto`) command converts the old rr0.3 DB layout (RLP) to the rr0.4 DB layout (Protobuf).
To guarantee faithful replay after upgrading, `db-rr0.3-to-rr0.4` replays the converted substates before writing them to the new substate DB.
`--blockchain` option can be used to supplement tx types which are required for rr0.4 substates but missing in rr0.3 substates.
```
./substate-cli db-rr0.3-to-rr0.4 --old-path rr0.3.substate.ethereum --new-path rr0.4.substate.ethereum --blockchain 0-1M.blockchain --block-segment 0-1M --workers 0
```

If `--blockchain` is not provided or substates are not found in the provided blockchain file, then `substate db-rr0.3-to-rr0.4` will guess tx types based on access lists and dynamic gas fees.
```
./substate-cli db-rr0.3-to-rr0.4 --old-path rr0.3.substate.ethereum --new-path rr0.4.substate.ethereum --block-segment 0-1M --workers 0
```

### `db-clone`
`substate-cli db-clone` command reads substates of a given block range and copies them in a substate DB clone.
```
./substate-cli db-clone --src-path srcdb --dst-path dstdb --block-segment 1-2M --workers 0
```

### `db-compact`
`substate-cli db-compact` command runs compaction functionality of the backend of the given substate DB.
```
./substate-cli db-compact --substatedir substate.ethereum
```

### `db-export`
`substate-cli db-export` command exports substates to files.
`--json` option to export base64 JSON instead of binary.
`--hashed` option to replace raw bytecode to code hash when export.
For hashed substates, read through the next sections for more details.
```
./substate-cli db-export --substatedir substate.ethereum --out-dir substate-db-export --block-segment 1-2M --workers 0
```
The exported files are named after their block number and tx index.
For example, the substate file at tx index 0 at block 1,000,000 has `1000000_0` in its name.



## Substate data structures
Since `rr0.4`, the substate recorder and replayer use [Protobuf](https://github.com/protocolbuffers/protobuf) for serialization for better compatibility and productivity with Go and other major programming languages such as C++, Java, and Python.
Until `rr0.3`, the substate recorder and replayer used Ethereum RLP encoding for serialization of substate data structures.

To use Protobuf, it is required to compile Protobuf definition to the source code of Go or other languages.
For Protobuf support in Go, install both `protoc` and `protoc-gen-go` as described in [this link](https://protobuf.dev/getting-started/gotutorial/#compiling-protocol-buffers).
Executing `./protoc-substate.sh` will read `substate.pb` and generate `substate.pb.go` file.

Protobuf named its data structure as *message*.
`Substate` defines the following 5 messages:
1. `Account`: account information (nonce, balance, one of code or code hash, storage)
2. `Alloc`: mapping of account addresses and `Account` messages.
3. `BlockEnv`: information from block headers (block gas limit, number, timestamp, block hashes)
4. `TxMessage`: transaction message for execution
5. `Result`: result of transaction execution

`Substate` contains the following 5 values required to replay transactions and validate results:
1. `input_alloc`: alloc that is read during transaction execution
2. `block_env`: block information required for transaction execution
3. `tx_message`: array with exactly 1 transaction
4. `output_alloc`: alloc that is generated by transaction execution
5. `result`: execution result and receipt array with exactly 1 receipt

[substate_utils.go](./substate_utils.go) defines helper functions to convert between data structures in Geth and Protobuf.

### Unhashed substate vs. Hashed Substate
Contract code in `Account` and initialization code in `TxMessage` are defined as `oneof` raw bytecode (i.e., unhashed) or Keccak256 code hash (i.e., hashed) in [substate.proto](./substate.proto).
The main purpose of converting unhashed substates to hashed substates is to reduce the size of substates by replacing lengthy bytecode with fixed-size code hash.

`substate_utils.go` defines `HashMap`, `HashKeys`, `HashedCopy`, and `UnhashedCopy` helper functions for conversion between unhashed substates and hashed substates.



## Tips for debugging recorder
You may want to modify our recorder to capture more information into substates.
Our recorder tests faithful transaction replay with every substate before it writes them to substate DB.
If this test fails, the recorder immediately stops and stores substates into JSON files named after `block_tx`.
All byte arrays in the substate JSON files are encoded in *Base64* (unlike Geth using hex for bytes in its JSON files).
You can manually inspect what makes the replay test fail using `diff` command, or programmatically by writing a script that compares the JSON files using JSON libraries.



## Tips for debugging replayer
You may instrument EVM in our replayer instead of the P2P client to speed up dynamic analysis on EVM bytecode.
In this case, modify and run `substate-cli replay` which checks the EVM output with the recorded output.
If those two outputs are different in `substate-cli replay`, it will print substates formatted in JSON and terminate.
In this case, use `--workers=1` option for sequential transaction execution and identify the block N that causes the problem.
Then, add code that prints values for debugging or use debugging tools.

You may modify EVM specification and want to see the effects of the updates.
It also means that you don't expect that all transactions are faithfully replayed.
In this case, modify and run `substate-cli replay-fork` which checks differences in the outputs and reports them.
