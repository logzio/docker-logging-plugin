
# Logz.io Docker logging plugin
This Docker plugin ships container logs to your Logz.io account. These instructions are for Linux host systems. For other platforms, see the [Docker Engine managed plugin system documentation](https://docs.docker.com/engine/extend/).

# Prerequisites
Here's what you need to run the plugin:

* Docker Engine version 17.05 or later. If you plan to configure this plugin using `daemon.json`, you need Docker Community Edition (Docker-ce) 18.03 or later.
* A [Logz.io](https://logz.io) account

# Install and configure the Logz.io Docker logging plugin

## Step 1: Install the plugin
Choose how you want to install the plugin:
* [Option 1: Install from the Docker Store](#option-1-install-from-the-docker-store)
* [Option 2: Install from source](#option-2-install-from-source)


### Option 1: Install from the Docker Store

1. Pull the plugin from the Docker Store:
  ```
  $ docker plugin install logzio/Docker-Logging-Plugin:latest --alias logzio/logzio-logging-plugin
  ```

2. Enable the plugin, if needed:
  ```
  $ docker plugin enable logzio/logzio-logging-plugin
  ```

Continue to [Step 2: Set configuration variables](#step-2-set-configuration-variables)

### Option 2: Install from source

1. Clone the repository and check out release branch:
  ```
  $ git clone https://github.com/logzio/Docker-Logging-Driver.git
  $ cd Docker-Logging-Driver
  $ git checkout release
  ```

2. Build the plugin:
  ```
  $ make all
  ```

3. Enable the plugin:
  ```
  $ docker plugin logzio/logzio-logging-plugin enable
  ```

4. Restart the docker daemon for the changes to apply:
  ```
  $ service docker restart
  ```

Continue to [Step 2: Set configuration variables](#step-2-set-configuration-variables)

## Step 2: Set configuration variables
Choose how you want to configure the plugin parameters:
* [Option 1: Configure all containers with daemon.json](#option-1-configure-all-containers-with-daemonjson)
* [Option 2: Configure individual containers at run time](#option-2-configure-individual-containers-at-run-time)


### Option 1: Configure all containers with daemon.json
The `daemon.json` file allows you to configure all containers with the same options.

For example:
```
{
  "log-driver": "logzio/logzio-logging-plugin",
  "log-opts": {
    "logzio-url": "<logzio_account_token>",
    "logzio-token": "<logzio_account_url>",
    "logzio-dir-path": "<dir_path_to_logs_disk_queue>"
  }
}
```

1. _(Optional)_ Set any [environment variables](#advanced-options-environment-variables)
2. Include all [required variables](#required-variables) in your configuration and any [optional variables](#optional-variables).

Continue to [Step 3: Run containers](#step-3-run-containers)

### Option 2: Configure individual containers at run time

Configure the plugin separately for each container when using the docker run command. For example:
```
$ docker run --log-driver=logzio/logzio-logging-plugin --log-opt logzio-token=<logzio_account_token> --log-opt logzio-url=<logzio_account_url> --log-opt logzio-dir-path=<dir_path_for_logs_disk_queue> <your_image>
```

1. _(Optional)_ Set any [environment variables](#advanced-options-environment-variables)
2. Include all [required variables](#required-variables) in your configuration and any [optional variables](#optional-variables).

Continue to [Step 3: Run containers](#step-3-run-containers)


#### Required Variables

| Variable | Description | Notes |
| --- | --- | --- |
| `logzio-token` | Logz.io account token. | |
| `logzio-url` | Logz.io listener URL. For the EU region, use `https://listener-eu.logz.io:8071`. Otherwise, use `https://listener.logz.io:8071`. | To find your region, look at your login URL. `app.logz.io` is US. `app-eu.logz.io` is EU. |
| `logzio-dir-path` | Logs disk path. All the unsent logs are saved to the disk in this location. | |

#### Optional Variables

| Variable | Description | Default value |
|---|---|---|
| `logzio-source` | Event source | |
| `logzio-format` | Log message format, either `json` or `text`. | `text` |
| `logzio-tag` | See Docker's [log tag option documentation](https://docs.docker.com/v17.09/engine/admin/logging/log_tags/)	| `{{.ID}}` (12 characters of the container ID) |
| `labels` | Comma-separated list of labels to be included in the log message. | |
| `env` | Comma-separated list of environment variables to be included in message. | |
| `env-regex` | A regular expression to match logging-related environment variables. Used for advanced log tag options. If there is collision between the `label` and `env` keys, `env` wins. Both options add additional fields to the attributes of a logging message. | |
| `logzio-attributes` | Meta data in a json format that will be part of every log message that is sent to Logz.io account. | |

#### Advanced options: Environment Variables

| Variable | Description | Default value |
|---|---|---|
| `LOGZIO_DRIVER_LOGS_DRAIN_TIMEOUT` | Time to sleep between sending attempts | `5s`
| `LOGZIO_DRIVER_DISK_THRESHOLD` | Above this threshold (in % of disk usage), plugin will start dropping logs | 	`70` |
| `LOGZIO_DRIVER_CHANNEL_SIZE` | How many pending messages can be in the channel before adding them to the disk queue. | `10000` |
| `LOGZIO_MAX_MSG_BUFFER_SIZE`	| Appends logs that are segmented by docker with 16kb limit. It specifies the biggest message, in bytes, that the system can reassemble. 1 MB is the default and the maximum allowed. | `1048576` (1 MB) |

### Usage example

```
$ docker run --log-driver=logzio/logzio-docker-logging-plugin \
             --log-opt logzio-token=123456789 \
             --log-opt logzio-url=https://listener.logz.io:8071 \
             --log-opt logzio-tag="{{.Name}}/{{.FullID}}" \
             --log-opt labels=region \
             --log-opt env=DEV \
             --env "DEV=true" \
             --label region=us-east-1 \
             <docker_image>
```

## Step 3: Run containers

Now that the plugin is installed and configured, it will send the container while the container is running.

To run your containers, see [Docker Documentation](https://docs.docker.com/config/containers/logging/configure/).

## Credits
This plugin relies on the open source [Logz.io go https shipper](https://github.com/dougEfresh/logzio-go) by [Douglas Chimento](https://github.com/dougEfresh)
