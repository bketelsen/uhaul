# uhaul

Build systemd-sysexts in a custom prefix by patching binaries and libraries.

Don't use this. It's still very naive and probably will eat your data and insult your family name.


## Usage

### Building

```bash
go build
```

### Running

Create the output/prefixed directories and patch the bin/libs.

```bash
./uhaul --clean nvim
```

Create the sysext

```bash
./sysext.sh out
```


