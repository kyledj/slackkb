# Slack Killboard Bot

Polls the zkillboard API for new kills and posts them to Slack.

In the simplest case:

```bash
go build slackkb.go
./slackkb.go
```

This will use the default `config.json` file.

## Config

The config file contains the following parameters:

### zkurl

The [Zkillboard API](https://neweden-dev.com/ZKillboard_API) URL you want
to follow.The URL provided is an example which grabs kills for our corp,
[The Desolate Order](http://desolateorder.com/forum/). You can find the ID
of the thing you care about by searching the normal ZKillboard dashboard.
Please note, this bot naively adds a `startTime` parameter to the path, so
the URL you use in this config must be compatible.
   
### channel

The Slack channel you want the bot posting to

### slackbot_url

This is the URL provided by the Slackbot Integration.
[See this page for details](https://api.slack.com/slackbot)


## Running

There are a few simple parameters the bot can use

### config

`-config=myconf.json`

Specify the path to the config file

### ignore

`-ignore=ignore.txt`

Specify the path to a list of systems which should be ignored. These were
included for our case to ignore BRAVE home system spam. An example of the
SQL used to query the static DB for these systems is included in
`getignored.sql`

Note that kills over 1 billion isk (if zkb provides the value) will be included
regardless of ignore status, because those are awesome.

### test

`-test`

Runs in "testing" mode. Will only log kills it would post, but not send them to slack.

## Errata

I experimented with different forms of Slack integration: a custom integration, extra data, etc, and what I found was that the default embedding from a normal Slackbot post turned out better than anything I could easily manage with other methods. You may have a better experience with a custom integration.

