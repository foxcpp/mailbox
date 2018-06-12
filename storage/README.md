## Configuration storage

Mailbox uses following directory structure where `$XDG_CONFIG_HOME` is (and
defaults to) `~/.config` on Linux and `%USERPROFILE%\AppData\Roaming` on Windows.

```
$XDG_CONFIG_HOME/
 mailbox/
  plugins/
   pluginA.so
   pluginA.yml
  accounts/
   fox.cpp@disroot.org-ebb30a58c5a1.yml
   another@disroot.org-5a160904e35e.yml
  frontends/
   cli.yml
   gui.yml
  global.yml
```

Directory `mailbox/` with all files and subdirectories are watched for changes
and reloaded automatically. File separation allows to do a partial
configuration reload on change.
Configuration files updated instantly after changes in running program.

Configuration files do not have any kind of version, default values for new
fields are just added to old configuration.


### global.yml

`global.yml` have following scheme:
```
connection:
  # Maximum amount of attempts to connect to IMAP/SMTP server before aborting.
  connection_tries: 5
```

### frontends/

Files in `frontends/` directory store frontend-specific configuration and not
described here.

### accounts/

Files in `accounts/` directory define per-account global overrides, server
connection configuration and credentials.

File name does not matter really but used as an ID for account. Core implementation
generates them in following format:
```
email-random.yml
```
Where `random` is 6 hex-encoded random bytes.

Each file in this directory have following schema:
```
name: "fox.cpp" # Name used in From field
server:
  imap:
    host: mail.disroot.org
    port: 993
    encryption: tls # can be "tls" or "starttls"
  smtp:
    host: mail.disroot.org
    port: 587
    encryption: starttls
credentials:
  user: fox.cpp
  pass: "47a7378384f36416e72:48716d2f713076513279544a6f426877646a436948776755647470" # see below
anyothersection:
  # overrides for anyothersection from global.yml
```

#### Password encryption

Passwords stored encrypted using AES-256 in CBC mode with key generated using
[go-sysid] library with SHA-256 hash. IV is stored together with encrypted
password (before colon).  Both IV and encrypted password are hex-encoded.

Password may be absent, in this case empty string should be encrypted and
frontend should ask user for password each time.

### plugins/

Files in `plugins/` directory contain native plugins themselves and matching
configuration files. No explicit schema is defined (plugins feel free to write
any YAML in it).


[go-sysid]: https://github.com/foxcpp/go-sysid
