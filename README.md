# Packer Docker build post processor

[![Build Status](https://travis-ci.org/avishai-ish-shalom/packer-post-processor-docker-dockerfile.svg)](https://travis-ci.org/avishai-ish-shalom/packer-post-processor-docker-dockerfile)

This is a [packer](http://packer.io/) post processor plugin which allows setting Docker metadata on Docker artifact.

Normally, Docker iamges built using packer cannot include user, environment variables and other metadata that is available using Dockerfiles.

This plugin will automatically create a temporary Dockerfile and run `docker build` in an annonymous context. Most Dockerfile instructions are supported as json parameters, note that `ADD`, `RUN`, `COPY` etc. are not supported because packer provisioners should be used for this functionality.

## Usage

In your packer template, configure the post processor:

```json
{
  "post-processors": [
    {
      "type": "docker-dockerfile",
      "env": { "envname": "envvalue" },
      "user": "userid",
      "volume": ["/data"],
      "entrypoint": ["/bin/bash", "-v"],
      "expose": [8080, 8081]
    }
  ]
}
```

Values can include user variables and other packer functions as documented on the packer manual.

`cmd` and `entrypoint` can have either array or string values, this mirrors Dockerfile format and functionality; See the docker build reference for details.

Please note that if you are using the `docker-tag` post processor to tag the resulting artifact of this post processor then you must put both post processor on the same chain:

```json
{
...
  "post-processors": [
    [
      {
        "type": "docker-dockerfile",
        "env": {
          "SOMEENVVAR": "value"
        },
        "user": "userid",
        "volume": ["/data", "/logs"]
      },
      {
        "type": "docker-tag",
        "repository": "packer/whatever",
        "tag": "latest"
      }
    ]
  ]
...
}
```

## Building

It's recommended to build this plugin using `goop`. Install `goop`:

    go get github.com/nitrous-io/goop && go build github.com/nitrous-io/goop

Then build the packer plugin. From within the plugin source code directory use the commands:

    goop install && goop go build

Copy the binary `packer-post-processor-docker-dockerfile` to your packer directory.

## License

This plugin is released under the Apache V2 license

## Support

Please file an issue on the github repository if you think anything isn't working properly or an improvement is required.

This plugin has been tested with packer 0.7.2 and 0.8 development branch.
