#!/bin/sh
docker rmi zy/listener
docker build -t zy/listener .
