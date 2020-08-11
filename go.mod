module github.com/matheusd/tspend

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/blockchain/stake/v3 v3.0.0-20200623174822-e2d77e4e7efe
	github.com/decred/dcrd/blockchain/standalone/v2 v2.0.0-00010101000000-000000000000
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v3 v3.0.0-20200608124004-b2f67c2dc475
	github.com/decred/dcrd/dcrec/secp256k1/v3 v3.0.0-20200616182840-3baf1f590cb1
	github.com/decred/dcrd/dcrutil/v3 v3.0.0-20200616182840-3baf1f590cb1
	github.com/decred/dcrd/rpcclient/v6 v6.0.0-20200616182840-3baf1f590cb1
	github.com/decred/dcrd/txscript/v3 v3.0.0-20200623174822-e2d77e4e7efe
	github.com/decred/dcrd/wire v1.3.0
	github.com/decred/slog v1.0.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/kr/pretty v0.1.0 // indirect
	github.com/onsi/ginkgo v1.10.2 // indirect
	github.com/onsi/gomega v1.7.0 // indirect
	golang.org/x/crypto v0.0.0-20190308221718-c2843e01d9a2
	golang.org/x/net v0.0.0-20190620200207-3b0461eec859 // indirect
	golang.org/x/sys v0.0.0-20191024172528-b4ff53e7a1cb // indirect
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
	gopkg.in/yaml.v2 v2.2.2 // indirect
)

replace (
	github.com/decred/dcrd/blockchain/stake/v3 => github.com/marcopeereboom/dcrd/blockchain/stake/v3 v3.0.0-20200811120412-2ef6149ec155
	github.com/decred/dcrd/blockchain/standalone/v2 => github.com/marcopeereboom/dcrd/blockchain/standalone/v2 v2.0.0-20200714223207-99f55c5b62bb
	github.com/decred/dcrd/chaincfg/v3 => github.com/marcopeereboom/dcrd/chaincfg/v3 v3.0.0-20200811120412-2ef6149ec155
	github.com/decred/dcrd/txscript/v3 => github.com/marcopeereboom/dcrd/txscript/v3 v3.0.0-20200811120412-2ef6149ec155
	github.com/decred/dcrd/wire => github.com/marcopeereboom/dcrd/wire v0.0.0-20200714223207-99f55c5b62bb
)
