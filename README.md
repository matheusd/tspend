# Create Treasury TSpends

Requires go > 1.13.

```shell
#  Don't Panic
$ go run . -h

# Generate TSpend for simnet without an underlying dcrd
# (you need to know the correct expiry)
$ go run . \
  --simnet \
  --privkey "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" \
  --address "SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx" \
  --amount 1075000000 \
  --expiry 386

# Generate and publish a TSpend for simnet
# (uses the standard one from the dcrd tmux script)
$ go run . \
  --simnet \
  -u USER -P PASS \
  --privkey "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" \
  --address "SsnhVyWxY6c5xEztSBb9xBqf9gdjEHpyCDx" \
  --amount 1075000000 \
  --publish
```

Using with an encrypted private key. This demo requires [age](https://github.com/FiloSottile/age).

```shell
# Create the encrypted key file however you want.
echo "62deae1ab2b1ebd96a28c80e870aee325bed359e83d8db2464ef999e616a9eef" | age -p simnet-pi-key.age

# Generate a tspend while decrypting from the file.
age -d simnet-pi-key.age | go run . --simnet \
  --expiry 386 \
  --address="SsinZEEYLHmyz3NXjijcJ2HFy6zvh4VyKLR" \
  --amount 1063000000
```
