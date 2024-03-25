# Ethereum Substate Recorder/Replayer TODO

* Check [README.md](./README.md) for docs.
* Check [CHANGELOG.md](./CHANGELOG.md) for release notes.

When a TODO item is completed, move it to a release note in CHANGELOG.md



## New database backends
Priority: high

Goleveldb is not actively maintained. It is not compatible with the official LevelDB C++ implementation.
* Option 1: Embedded KVDB. The main advantage is straightforward migration from Goleveldb to a new KVDB backend. Geth changed its backend from Goleveldb to Pebble, a RocksDB implementation in the Go language. Erigon (Turbo-Geth in the past) uses MDBX (a derivative of LMDB) which has good Go and Python libraries.
* Option 2: RDBMS and SQL. The main advantage is portability and compatibility because major languages have SQL libraries. If a new RDBMS backend supports concurrency very well, multiple recorders and/or replayers can run in parallel on multicore and/or distributed systems. Embedded RDBMS such as SQLite3, or remote RDBMS such as MySQL, MariaDB, and PostgreSQL.
* Option 3: a DB server with support of public APIs such as REST, GraphQL, gRPC, etc.

Introduce new `--substate-db` option which receives `"backend,URI"` parameter. Deprecate `--substatedir` flag in favor of the new `--substate-db`. For example:
```
--substate-db "goleveldb,substate.ethereum"
--substate-db "pebble,/path/to/substatedir"
--substate-db "mdbx,/path/to/substates_mdbx"
--substate-db "sqlite3,file://path/to/db.sqlite3"
--substate-db "mysql,root:pwd@tcp(127.0.0.1:3306)/testdb"
--substate-db "postgres,user=pqgotest dbname=pqgotest sslmode=verify-full"
```



## New substate DB layout
Priority: NOT recommended!

*A new substate DB layout to save metadata is not recommended!*
* For example, get tx types, account addresses, code hashes, and init code hashes before substate deserialization. This requires major changes in the design and implementation of hashed and unhashed substates.
* Not sure whether changing substate DB layout for metadata benefits the overall off-the-chain testing framework. Changing the DB layout breaks the replayer's backward compatibility. This requires upgrading the entire DB or recording billions of substates again.
* With Protobuf and new DB backends with better portability with other languages, it will become much simpler to write client programs to iterate all substates and collect metadata. This is a more recommended way as it preserves forward and backward compatibilities of substate DB.



## record-replay based on another Ethereum client
Priority: moderate

Ethereum community thinks that client diversity is critical.
https://clientdiversity.org/#why

In the point of testing framework, client diversity is not critical. However, there are other reasons to consider working based on clients other than Geth. Some clients are optimized for stability, speed, and/or data size.

Geth is not very fast and requires a huge amount of space. To solve the speed and space problems, Geth frequently changes the DB scheme every year. But Geth never provides an offline DB migration tool. (The complexity of guaranteeing the correctness of historical states after migration seems to be challenging.) The only option is fresh DB synchronization from scratch again - either import the entire chain again for several weeks or wait for some latest Geth nodes to fully synced to provide the snapshot over the network.

There are two options when implement recorder/replayer based on other Ethereum execution layer (EL) clients.
* Option 1: A new client can replay transactions in full sync importing blocks from chain files, e.g., `geth import`.
* Option 2: A new client can replay transactions via JSON RPC on archive nodes, e.g., `debug_traceBlockByNumber`. We can add a new JSON RPC function e.g., `substate_recordBlockSubstates` based on `debug_traceBlockByNumber` which returns recorded substates of a given block number.
  * With a substate DB backend that other languages can read and write, it becomes possible to keep an archive node running to sync the latest blocks and use the JSON RPC client to call `substate_recordBlockSubstates` and save returned substates to the substate DB.

Erigon and Reth are optimized for time and space to maintain EL nodes compared to other clients including Geth, Nethermind, and Besu.

Erigon (Turbo-Geth) is based on Geth but much faster in full sync than other clients with smaller archive node sizes.  Erigon embedded its own consensus layer client called Caplin used when `--internalcl` option is given.
* Option 1: `erigon import` can be used. The current latest versions of Erigon, 2.58.2 and 2.59.0, suffer segmentation faults with `erigon import` command with the initial 100k blocks (`1-1000000`).
* Option 2: Erigon archive node is much smaller and faster, taking about a week to sync 15M blocks. Erigon supports `debug_traceBlockByNumber`.

Reth is a recently started project to implement EL client in Rust programming language. Reth manages its EVM implementation as the external revm crate, which needs some time to work on manual instrumentation in revm and modify reth build config to use the instrumented revm. Reth is still a beta version. Reth supports JSON RPC APIs and EVM instruction tracers similar to the Geth-style. If Reth defines all JSON RPC APIs and EVM tracers in the reth crate itself, we may not need to spend time for instrumenting revm.
* Option 1: `reth import` command can be used. Need to test whether the latest Reth has no problem running `reth import`.
* Option 2: Reth supports Geth-style JSON RPC APIs and EVM tracers for `debug_traceBlockByNumber`.
