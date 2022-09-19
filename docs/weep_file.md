## weep file

Retrieve credentials and save them to a credentials file

### Synopsis

The file command writes role credentials to the AWS credentials file, usually 
~/.aws/credentials. Since these credentials are static, you’ll have to re-run the command
every hour to get new credentials.

More information: https://hawkins.gitbook.io/consoleme/weep-cli/commands/credential-file


```
weep file [role_name] [flags]
```

### Options

```
  -f, --force            overwrite existing profile without prompting
  -h, --help             help for file
  -o, --output string    output file for credentials (default "/Users/mrowland/.aws/credentials")
  -p, --profile string   profile name (default "default")
  -R, --refresh          automatically refresh credentials in file
```

### Options inherited from parent commands

```
  -A, --assume-role strings        one or more roles to assume after retrieving credentials
  -c, --config string              config file (default is $HOME/.weep.yaml)
      --extra-config-file string   extra-config-file <yaml_file>
      --log-file string            log file path (default "/tmp/weep.log")
      --log-format string          log format (json or tty)
      --log-level string           log level (debug, info, warn)
  -n, --no-ip                      remove IP restrictions
  -r, --region string              AWS region (default "us-east-1")
```

### SEE ALSO

* [weep](weep.md)	 - weep helps you get the most out of ConsoleMe credentials

