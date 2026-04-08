# LTC-Wallet

A portable cold Litecoin wallet for Windows. No installation required. No seed phrase to store.

## The idea

Most cold wallets make you write down a seed phrase and keep it safe forever.
This one works differently — your private key is never stored anywhere.
Instead, it is derived on the fly from a sequence of actions called a **ritual**.
Same actions, same parameters — same key. Every time.

Built on [Ritual Protocol](https://github.com/runetcom2014/ritual-protocol).

## Features

- Portable — single executable, runs from a USB drive, leaves nothing on the machine
- No seed phrase — nothing to write down, nothing to steal
- Shows balance and transaction history via public Litecoin nodes
- Send LTC transactions
- Built with Go + Fyne

## How to build

Requirements: Go 1.21+, GCC (MinGW)

```
git clone https://github.com/runetcom2014/LTC-Wallet
cd LTC-Wallet
go build -o LTC-Wallet.exe .
```

Place `bin/ritual.dll` in the same directory as `LTC-Wallet.exe` and run.

## Security note

This is a **demo application**, not production software. It demonstrates the Ritual Protocol concept using Litecoin. Use at your own risk. Do not store significant funds.

## Under the hood

The wallet uses Ritual Protocol to derive a 32-byte key from your ritual. That key is used as the wallet seed to generate a standard LTC address. The key exists only in memory during the session and is never written to disk.

Full protocol specification: [Ritual Protocol on Zenodo](https://zenodo.org/records/19090391)
