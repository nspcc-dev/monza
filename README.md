# Monza

Application for searching notifications in N3 compatible chains.


## Features

- Monza fetches and caches chain blocks in the filesystem, so application will
  not download it again at restart
- Monza manages different caches for different chains based on the magic number
- For NEP and NeoFS notifications monza produces detailed output
- Use relative numbers for search interval
- Use nice names for native contracts
- Search multiple notifications at once

## Examples

```
$ monza run -r https://rpc01.morph.testnet.fs.neo.org:51331 --from 466858 --to p1 -n NewEpoch:* -n Transfer:gas
syncing 100% [##################################################] (1/1, 1 blocks/s)
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:375dce88b11cc120e4b80a94549690ec67145e0f to:nil amount:2526059790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:0d967edfde7096d35d607177285286ba66f12fa9 to:nil amount:2525915790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:176b57a5107c9c96d49fe34b9378bf5bc4571249 to:nil amount:2525987790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:94a887b5ce941a32a5afcfc6e5630b3d11af17c0 to:nil amount:2525951790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:ff6cb3d4245193b2cb1b0047113252c8bce88661 to:nil amount:2526095790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:2492cbb531f39a422c00b8b00313f74a85e828d0 to:nil amount:2525879790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:4d91cf6cea3827f5d5694e668ab36b6e9c0b9aad to:nil amount:2526023790
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:nil to:2492cbb531f39a422c00b8b00313f74a85e828d0 amount:8291640
block:466858 at:2021-11-28T19:57:08+03:00 name:Transfer from:nil to:2492cbb531f39a422c00b8b00313f74a85e828d0 amount:50000000
block:466858 at:2021-11-28T19:57:08+03:00 name:NewEpoch epoch:1937
```

### Detailed output

- NEP-17 `Transfer`
```
block:956 at:2021-09-08T20:17:31+03:00 name:Transfer from:d08c4c71c779f1e3edb04dc6e434ad5bdac3f10f to:0df7f74adea1bd25011dfefb08949a27c250b640 amount:1218331468
```
- NeoFS `NewEpoch`
```
block:615896 at:2021-12-24T18:17:38+03:00 name:NewEpoch epoch:2558
```
- NeoFS `AddPeer`
```
block:615657 at:2021-12-24T17:17:50+03:00 name:AddPeer pubkey:[..8c0f7f] endpoints:[/dns4/st01.testnet.fs.neo.org/tcp/8080]
```

### Notifications

Search notifications based on notification name and contract address.
```
monza run -r [endpoint] --from 110000 --to 110100 -n NewEpoch:ab8a83432af3cd32ce6ba3797f62b1ba330d7c3d
```

Use wildcard to search notifications from any contract.

```
monza run -r [endpoint] --from 110000 --to 110100 -n NewEpoch:*
```

You can specify native contract names such as `gas` and `neo`.

```
monza run -r [endpoint] --from 110000 --to 110100 -n Transfer:gas
```

Specify multiple notifications to look for.

```
monza run -r [endpoint] --from 110000 --to 110100 -n Transfer:gas -n NewEpoch:*
```

### Intervals

Define start and stop blocks.

```
monza run -r [endpoint] --from 110000 --to 110100 -n NewEpoch:*
```

Omit `--to` flag to search up to the latest block.

```
monza run -r [endpoint] --from 110000 -n NewEpoch:*
```

To look for `100` blocks before the latest block use prefix `m` (minus)

```
monza run -r [endpoint] --from m100 -n NewEpoch:*
```

To look for `100` blocks after specified `from` block, use prefix `p` (plus)

```
monza run -r [endpoint] --from 101230 --to p100 -n NewEpoch:*
```

### Other

Blocks are stored in bolt databases. Specify dir for cache with `-c` flag
(default path is `$HOME/.config/monza`)

```
monza run -r [endpoint] -c ./cache --from 110000 --to 110100 -n NewEpoch:*
```

To speed up block fetching from the RPC node, use more parallel workers with
`-w` flag.

```
monza run -r [endpoint] -w 10 --from 110000 --to 110100 -n NewEpoch:*
```

To disable progress bar use `--disable-progress-bar` flag.


## Build

Use `make build` command. Binary will be stored in `./bin/monza`.


## To Do
- [ ] `monza cache` command to manage bbolt instances: provide size and option to delete
- [ ] Add verbose flag with for detailed view of notification body
- [ ] Do not print progressbar if stdout is not console
- [ ] Add more native contract hashes aliases
- [ ] More NEP support (NEP-11?)

## License

Source code is available under the [MIT License](/LICENSE).
