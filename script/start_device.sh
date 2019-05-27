#!/bin/bash
# by orientlu

rm ./log -rf
[ -d ./log ] ||  mkdir ./log

ls ./gw-*-config.toml | while read gw
do
    nohup ../loracli -conf ${gw} > ./log/${gw}.log  2>&1 &
    echo ${!} >> ${gw}.pid
done
