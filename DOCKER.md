# Building and running DASTARD in a Docker container

Joe Fowler. July 18, 2025

The included `Dockerfile` allows you to build, test, and run DASTARD within a Docker container. As
of this writing (July 18, 2025), Docker builds are experimental, but they were shown to work on
both Mac OS 15 (with Apple processors) and Ubuntu Linux.

It is still possible to install DASTARD outside of Docker, in which case you'd want to ignore this
file and the `Dockerfile`.


## Install Docker

The instructions for installing [Docker Desktop on Ubuntu Linux](https://docs.docker.com/desktop/setup/install/linux/ubuntu/) worked on Ubuntu 22.04.

1. Go to [Docker Desktop release notes](https://docs.docker.com/desktop/release-notes/) and find the latest Debian. Download the `docker-debian-amd64.deb` file.
2. Then install it:

```bash
sudo apt-get update
sudo apt-get install ./docker-desktop-amd64.deb
```

## Build an image for Dastard

From the DASTARD main directory:

```bash
docker build --tag dastard .
```

To run the tests instead

```bash
docker build --tag dastard-tests --target run-test-stage .
```

In docker desktop, go to Settings / Resources and select _Enable host netowrking_. (You'll have to restart Docker.)

## Run Dastard

```bash
# To run the usual way.
# The --rm means the container will be automatically deleted when it's done running.
# The --net=host will make the container share an IP address space with the host system.
# The --mount type=bind... will bind the directory ~/.dastard on the host to the container.
docker run --rm --net=host --mount type=bind,src=${HOME}/.dastard,dst=/root/.dastard dastard

# To run the version or help 
docker run dastard -version
docker run dastard -help

# To run a bash shell within the container, you must override the default entrypoint
docker run -it --entrypoint /bin/bash dastard
```

The run command's `--mount` argument will mount a host directory (`~/.dastard/`) into the container.
This binding ensures that any dastard configuration changes will both persist AND be available
for examination and change outside the container. They will even be shared between Dastard instances run in a 
container and outside of it.

### To do

* The command arguments required to run Dastard in a Docker container could be stored in a `docker-compose.yml` file. Docker
  compose is generally designed for using a bunch of containers in parallel, but we could set it up even for one container.
* Figure out how to interact with our Python programs, specifically dastard-commander and microscope.
