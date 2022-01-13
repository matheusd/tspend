# Create Treasury TSpends

Requires go >= 1.16.

## Quick Start

```shell
#  Don't Panic
$ go run . -h

# Generate TSpend for simnet without an underlying dcrd
# (you need to know the correct expiry)
$ go run . \
  --simnet \
  --privkey "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" \
  --address "SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx" \
  --amount 10.75000000 \
  --expiry 386

# Generate and publish a TSpend for simnet
# (uses the standard one from the dcrd tmux script)
$ go run . \
  --simnet \
  -u USER -P PASS \
  --privkey "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" \
  --address "SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx" \
  --amount 10.75000000 \
  --publish
```

## Private Key Encryption using ss 

Requires [ss](https://github.com/jrick/ss).

```shell
# Using ss and PKI encryption
echo "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" | ss encrypt -out ~/.tspend/simnet.key

# Using ss and passphrase encryption
echo "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" | ss encrypt -passphase -out ~/.tspend/simnet.key

# Generate a tspend while decrypting from the standard privkeyfile for the
# specified network. Also get values from a CSV and generate a sane expiry.
go run . --simnet -c 151 --csv in.csv
```


## Input/Output

Input TSpend payouts via CLI args or a CSV file.

Amounts are specified in **DCR**.

```shell
$ ... # rest of args
  --address SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx \
  --amount 10.75000000  \
  --address SsXBReLhVK8NrzZcBsu1Dyo5KhD19rgEcEv \
  --amount 8.53000000 
 
$ cat > input.csv
SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx,1075000000
SsXBReLhVK8NrzZcBsu1Dyo5KhD19rgEcEv,853000000 

$ ... # rest of args
  --csv input.csv
```

Raw tx in hex format is output to stdout. Redirect to an output file or use 
`--out` to save somewhere else. Use `--debuglevel` to tweak logging debug info.

```shell
$ ... # rest of args
  > out.txt

$ ... #rest of args
  --out tspend.hex

$ ... #rest of args
  --debuglevel=debug
```

## Config File

Add it to `~/.tspend/tspend.conf`:

```ini
[Application Options]

; Change the network
; testnet = 1
; simnet = 1

; Change the default fee rate of 10000
; feerate = 20000

; Change the privkeyfile used
; privkeyfile = ~/.tspend/[network].key

; Don't show debug info in stderr
; debuglevel = error
```

## Tests

Figure out the needed expiry for some block height.

```shell
go run ./expiryfor --simnet 240
```

## Voting Progress

Show voting progress for TSpends in the mempool:

```shell
go run ./voteprogress --simnet -u USER -P PASS
```
