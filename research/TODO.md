# Ethereum Substate Recorder/Replayer TODO

* Check [README.md](./README.md) for docs.
* Check [CHANGELOG.md](./CHANGELOG.md) for release notes.

When a TODO item is completed, move it to a release note in CHANGELOG.md



## New database backends
* Goleveldb is not actively maintained. It is not compatible with the official LevelDB C++ implementation.
  * Option 1: Embedded KVDB. The main advantage is straightforward migration from Goleveldb to a new KVDB backend. Geth changed its backend from Goleveldb to Pebble, a RocksDB implementation in the Go language. Erigon (Turbo-Geth in the past) uses MDBX (a derivative of LMDB) which has good Go and Python libraries.
  * Option 2: RDBMS and SQL. The main advantage is portability and compatibility because major languages have SQL libraries. If a new RDBMS backend supports concurrency very well, multiple recorders and/or replayers can run in parallel on multicore and/or distributed systems. Embedded RDBMS such as SQLite3, or remote RDBMS such as MySQL, MariaDB, and PostgreSQL.
  * Option 3: a DB server with support of public APIs such as REST, GraphQL, gRPC, etc.
* Introduce new `--substate-db` option which receives `"backend,URI"` parameter. Deprecate `--substatedir` flag in favor of the new `--substate-db`. For example:
```
--substate-db "goleveldb,substate.ethereum"
--substate-db "pebble,/path/to/substatedir"
--substate-db "mdbx,/path/to/substates_mdbx"
--substate-db "sqlite3,file://path/to/db.sqlite3"
--substate-db "mysql,root:pwd@tcp(127.0.0.1:3306)/testdb"
--substate-db "postgres,user=pqgotest dbname=pqgotest sslmode=verify-full"
```



## New substate DB layout (NOT recommended!)
* New substate DB layout to save metadata is not recommended!
  * For example, get tx types, account addresses, code hashes, and init code hashes before substate deserialization. This requires major changes in the design and implementation of hashed and unhashed substates.
* Not sure whether changing substate DB layout for metadata benefits the overall off-the-chain testing framework. Changing the DB layout breaks the replayer's backward compatibility. This requires upgrading the entire DB or recording billions of substates again.
  * With Protobuf and new DB backends with better portability with other languages, it will become much simpler to write client programs to iterate all substates and collect metadata. This is a more recommended way as it preserves forward and backward compatibilities of substate DB.



## record-replay based on another Ethereum client
* Recorder/Replayer implementation based on other Ethereum execution layer clients.
* Many reported that Erigon (Turbo-Geth in the past) and Reth are much faster in full sync than Geth, Nethermind, Besu. Reth is a bit faster than Erigon. Reth is a completely new implementation from scratch in Rust language. Erigon is based on Geth, so it will be much easier to migrate from Geth to Erigon than to Reth.
* A new client must be able to import blocks from a chain file in RLP exported from Geth. The full block import features of the new clients must be functional and stable enough.
  * Erigon 2.58.2 and 2.59.0 raise segmentation fault with `erigon import` command.
* If a new client supports archive node and JSON RPC for transaction replay like `debug_traceBlockByNumber`, then we can add a new RPC like `substate_recordBlockSubstates` to return substates in Protobuf JSON.
  * With a substate DB backend that other languages can read and write, it is possible to keep an archive node running to sync blocks and use a JSON RPC client calling `substate_recordBlockSubstates` and save returned substates to the substate DB.
  * This would work for Erigon which can sync via network sync but cannot import a chain file at the moment.
* Q. Create a new repository based on another Ethereum client, e.g., verovm/substate-recorder-erigon, or add the recorder based on Erigon to the existing verovm/record-replay repository?
