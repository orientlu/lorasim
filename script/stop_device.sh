#!/bin/bash
# by orientlu

if [ $# != 1 ]  || [ ! -f $1 ];then
    echo -n "usage\n\t$0 dev_list.pid\n\n  dev_list.pid is gen by prepare.py\n"
    exit 1
fi

while read line
do
    echo "kill device, pid:${line}"
    kill -9 ${line}
done < $1

rm $1 -f
