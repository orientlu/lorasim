#!/bin/bash
# by orientlu

ls ./gw-*-config.toml.pid | while read line
do
    pid=$(head $line)
    echo "kill device, pid:${pid}"
    kill -9 ${pid}
    rm ./${line}
done

