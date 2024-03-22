#!/bin/sh
# repoName, fullName, repoPath
dn=$(docker ps -aq --filter name=${1})
echo "Docker run and build start!"
echo "---------------------"
echo "docker stop $1"
echo "docker rm ${dn}"
echo "docker rmi "$2""
echo "docker build -t "$2" "$3""
echo "docker run -d --name "$1" -p 40030:80 "$2""

docker stop "$1"
docker rm ${dn}
docker rmi "$2"
docker build -t "$2" "$3"
docker run -d --name "$1" -p 40030:80 "$2"