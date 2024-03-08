# Ethereum Substate Recorder/Replayer Changelog



## Unstable release note

### TODOs
* Add a new version of `substate-cli db-upgrade` command to convert rr0.3 RLP substates to rr0.4 Protobuf substates.
  * Option 1: `db-upgrade` guesses tx types from the values of access lists and gas fees, as long as it guarantees faithful transaction replay.
  * Option 2: `db-upgrade` reads tx types from exported blockchain files.
* Update `substate-cli replay-fork` to support the latest hard forks in PoS era.
* A new database backend to replace Goleveldb. Goleveldb is not actively maintained. It is not compatible with the official LevelDB C++ implementation. A new DB backend should support in-database compression to keep billions of substates manageable. A new DB backend should show moderate write speed and fast read speed.
  * Option 1: Pebble. Geth changed its backend from Goleveldb to Pebble, a RocksDB implementation in the Go language.
  * Option 2: Any embedded database implementation that supports Go and other major languages (C++, Java, Python) and in-database compression.
  * Option 3: Any external/remote database with SQL or GraphQL query support. Geth has `--remotedb` option to access a remote read-only key-value database. Geth's [remotedb](https://pkg.go.dev/github.com/ethereum/go-ethereum/ethdb/remotedb) module uses `debug_dbGet` method. The main purpose of remotedb is debugging, not performance or scalability.
* Prepare for upcoming hard forks in 2024.
  * New hard forks require the latest Geth v1.13. The current record-replay is based on Geth v1.11.6 because Geth v1.12 has a problem importing PoW blocks. We must double-check whether Geth v1.13 can process PoW blocks. Otherwise, we need to use Geth v1.11 for PoW blocks and Geth 1.13 for PoS blocks in substate recording.

### DONE
* Add CHANGELOG.md
* Add a new version of `substate-cli db-upgrade` command to convert rr0.3 RLP substates to rr0.4 Protobuf substates.
With `--blockchain` option, `db-upgrade` reads tx types from exported blockchain files.
Without `--blockchain`, `db-upgrade` guesses tx types from the values of access lists and gas fees, as long as it guarantees faithful transaction replay.
* New `make record-replay` (or `make rr`) target for selectively faster compilation of recorder and replayer.



## record-replay 0.4.0 release note
**Full Changelog**: https://github.com/verovm/record-replay/compare/rr0.3.4...rr0.4.0

### Updates
* Put record-replay version as the postfix of `--version` information. For example, `geth --version` prints `geth version 1.11.6-rr0.4.0-commit` instead of `geth version 1.11.6-commit`.
* Use Protobuf 2 for `type Substate` definition to replace `type Substaet`, `type SubstateRLP`, and `type SubstateJSON`. Because Protobuf supports serialization to both binary and JSON, additional type definitions for serialization (`SubstateRLP`, `SubstateJSON`) are no longer required. Protobuf definition file `research/substate.proto` can be used for other languages.
* Support hard fork Paris a.k.a. "The Merge" for PoS support (`15_537_394`) and Shanghai (`17_034_870`).
* `substate-cli rr0.3-db-*` for legacy rr0.3 substate DB manipulation commands.
* The recorder runs the faithful replay test with the captured substates while recording. If the test fails, the recorder immediately stops and saves the problematic substates to JSON files to check.
* `substate-cli db-clone` and `substate-cli db-compact` with rr0.4 support
* Add a new command `geth record-substate` for recording substates and revert `geth import` to be same as the unmodified Geth without substate recording.

### Important notes
* Still based on Geth v1.11.6, because source code to import and process PoW blocks is deleted since Geth v1.12
* `rr0.4.0` does not provide `substate-cli db-upgrade` command to convert rr0.3 RLP substates to rr0.4 Protobuf substates. rr0.4 requires `Message.TxType` but rr0.3 does not have it. In the next release, there will be a newer version of `substate-cli db-upgrade` command which can guess tx types from access lists and gas fee values, or supplements tx types from Geth database, exported blockchain files, or text/CSV/JSON files.
* Still using Goleveldb as database backend. It requires more literature reviews and preliminary experiments to find a proper DB backend and DB schema.

### Faithful replay test
* rr0.4.0 recorder, `--block-segment 1-18_000_000` (`--block-segment 0-18M`): OK



## record-replay 0.3.4 Releate Note

**Full Changelog**: https://github.com/verovm/record-replay/compare/rr0.3.3...rr0.3.4

### Updates
* Support hard fork Gray Glacier (`15_050_000`).
* Based on Geth v1.11.6. (Geth dropped PoW support since v1.12 and the code to import PoW blockchain is completely removed.)
* Fix conflicts, cyclic imports, and issues from changes in Geth software architecture and APIs
* `--worker=0` to use the number of physical cores.
* `--block-segment` flag as a new way to pass block range.
* Refactored `substate-cli replay-*` and `substate-cli db-*` commands to use flags instead of positional arguments.
* `substate-cli replay` clears log fields that are not saved in DB and not used in comparison before reporting inconsistent output.

### Important Notes
* No support for hard fork Paris, a.k.a The Merge (`15_537_394`) for PoS in rr0.3.4.
* rr0.3.4 will be the last rr0.3 patch using goleveldb as DB backend and RLP for substate encoding. rr0.4 will come with different encoding and DB backend. (https://github.com/verovm/record-replay/issues/2 for more details.)

### Compatibility Test
Tested compatibility between rr0.3.4 replayer and substate DBs from recorder versions:
* rr0.2.0 recorder, `--block-segment 1-9_000_000` (`--block-segment 0-9M`): OK
* rr0.3.0 recorder, `--block-segment 9_000_001-15_000_000` (`--block-segment 9-15M`): OK
* rr0.3.4 recorder, `--block-segment 15_000_001-15_500_000` (`--block-segment 15000-15500k`): OK
* rr0.3.4 recorder, `--block-segment 15_500_001-16_000_000`: ERROR, rr0.3.4 recorder does not capture `BlockContext.Random` for The Merge for PoS.



## record-replay 0.3.3 release note

**IMPORTANT: 10 transactions recorded with rr0.2 in 9-10M blocks produce inconsistent output in rr0.3 replayer.** (https://github.com/verovm/record-replay/issues/1 for more details). The working solution is recording those 10 transactions again with rr0.3 recorder, or ignoring the last SSTORE key accessed in STATICCALL in those 10 transactions.

**Full Changelog**: https://github.com/verovm/record-replay/compare/rr0.3.2...rr0.3.3
* Improved docs and output messages



## record-replay 0.3.2 release note

**Full Changelog**: https://github.com/verovm/record-replay/compare/rr0.3.1...rr0.3.2

* Fix stats and messages from SubstateTaskPool.Execute
* Update README.md and --help messages



## record-replay 0.3.1 release note

* Fixed GetBlockSubstates() to decode legacy substates properly

**Full Changelog**: https://github.com/verovm/record-replay/compare/rr0.3.0...rr0.3.1



## record-replay 0.3.0 release note

* Based on Geth 1.10.15
* Support recording and replaying beyond London hard fork (block `#12,965,000`, fee market from EIP-1559)
* Support recording beyond Arrow Glacier hard fork (block `#13,773,000`)



## record-replay 0.2.0 release note

* Based on Geth v1.10.3
* New substate DB layout (use `substate-cli db upgrade` to upgrade from 0.1.0's)
* Support recording and replaying beyond Berlin hard fork (block `#12,244,000` access list from EIP-2930)
* Off-the-chain replayer optimization (~10% faster replay speed)



## record-replay 0.1.0 release note
Ethereum substate recorder/replayer based on the paper:

**Yeonsoo Kim, Seongho Jeong, Kamil Jezek, Bernd Burgstaller, and Bernhard Scholz**: _An Off-The-Chain Execution Environment for Scalable Testing and Profiling of Smart Contracts_,  USENIX ATC'21

Visit [verovm/usenix-atc21](https://github.com/verovm/usenix-atc21) repository for software artifact from the ATC'21 paper.
