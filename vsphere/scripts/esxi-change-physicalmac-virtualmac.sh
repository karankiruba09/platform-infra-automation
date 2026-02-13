#!/bin/sh
dvsName=<dvs_name>
portId=$(esxcfg-vswitch -l |grep vmk0 |awk '{print $1}')
ip=$(esxcli network ip interface ipv4 get -i vmk0 | grep vmk0 | awk '{print $2}')
mask=$(esxcli network ip interface ipv4 get -i vmk0 | grep vmk0 | awk '{print $3}')
gw=$(esxcli network ip interface ipv4 get -i vmk0 | grep vmk0 | awk '{print $6}')
esxcli network ip interface remove --interface-name=vmk0
esxcli network ip interface add --interface-name=vmk0 --dvs-name=${dvsName} --dvport-id=${portId}
esxcli network ip interface ipv4 set --interface-name=vmk0 --ipv4=${ip} --netmask=${mask} --gateway ${gw} --type=static
esxcfg-route -a default ${gw}