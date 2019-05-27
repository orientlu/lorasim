#!/bin/bash
# by orientlu

if [ $# != 1 ]  || [ ! -f $1 ];then
    echo -n "usage\n\t$0 dev_list\n\n  dev_list is gen by prepare.py\n"
    exit 1
fi

[ -d ./log ] ||  mkdir ./log

while read dev_env
do
    eval ${dev_env}
    nohup ../loracli -conf ../conf.toml > ./log/${GATEWAY_MAC}.log  2>&1 &
    echo ${!} >> $1.pid
    sleep  $(echo "$RANDOM%1000*0.001"|bc)
done < ${1}
