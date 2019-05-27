#!/usr/bin/env python
# coding=utf-8
# by orientlu

import lora_client
import toml
import os
import sys
import struct
import binascii

CFGFILE = "./config.toml"


def init_conf():
    if not os.path.exists(CFGFILE):
        input(CFGFILE + "not found")
        sys.exit(-1)
    with open(CFGFILE, mode='rb') as f:
        content = f.read()
    if content.startswith(b'\xef\xbb\xbf'):     # 去掉 utf8 bom 头
        content = content[3:]
    dic = toml.loads(content.decode('utf8'))
    return dic


def get_organizationsID(config):
    org = client.get_organizationsList(search=config['organizations']['name'])
    if org['totalCount'] == '0':
        org_id = client.create_organizations(
            config['organizations']['name'],
            config['organizations']['display_name'])
    else:
        org_id = int(org['result'][0]['id'])

    print("---> organization id :", org_id)
    return org_id


def setup_ns(client, nsConfig, org_id):
    print("start setup ns")
    nsConfig['id'] = None
    nsList = client.get_networkServersList()
    for ns in nsList['result']:
        if ns['name'] == nsConfig['name']:
            nsConfig['id'] = ns['id']
            print("---> use ns,", nsConfig['name'], " id: ", nsConfig['id'])
            break
    if nsConfig['id'] is None:
        nsConfig['id'] = client.add_networkServers(
            nsConfig['name'], nsConfig['server'])
        print("---> add ns,", nsConfig['name'], " id: ", nsConfig['id'])

    nsConfig['profiles_id'] = None
    nspList = client.get_serviceProfilesList()
    for nsp in nspList['result']:
        if nsp['name'] == nsConfig['profiles_name']:
            nsConfig['profiles_id'] = nsp['id']
            print("---> use ns_pf,", nsConfig['profiles_name'], " id: ",
                  nsConfig['profiles_id'])
            break
    if nsConfig['profiles_id'] is None:
        nsConfig['profiles_id'] = client.add_serviceProfiles(
            name=nsConfig['profiles_name'],
            nsID=nsConfig["id"],
            organID=org_id)
        print("---> add ns_pf,", nsConfig['profiles_name'], " id: ",
              nsConfig['profiles_id'])


def setup_gw(client, gwConfig, org_id, ns_id):
    print("start setup gw")
    gwConfig['profiles_id'] = None
    gwpList = client.get_gatewayProfilesList(nsID=ns_id)
    for gwp in gwpList['result']:
        if gwp['name'] == gwConfig['profiles_name']:
            gwConfig['profiles_id'] = gwp['id']
            print("---> use gw_pf,", gwConfig['profiles_name'], " id: ",
                  gwConfig['profiles_id'])
            break
    if gwConfig['profiles_id'] is None:
        gwConfig['profiles_id'] = client.add_gatewayProfiles(
            gwConfig['profiles_name'], ns_id, gwConfig['profiles_channels'])

        print("---> add gw_pf,", gwConfig['profiles_name'], " id: ",
              gwConfig['profiles_id'])

    needAdd = True
    gwList = client.get_gatewaysList(org_id)
    for gw in gwList['result']:
        if gw['id'] == gwConfig['id']:
            needAdd = False
            print("---> use gw,", gwConfig['name'], " id: ",
                  gwConfig['id'])
            break
    if needAdd:
        client.add_gateways(
            name=gwConfig['name'], gwID=gwConfig['id'],
            gwProfileID=gwConfig['profiles_id'],
            nsID=ns_id, organID=org_id)

        print("---> add gw,", gwConfig['name'], " id: ", gwConfig['id'])


def setup_device(client, devConfig, org_id, ns_id, app_id):
    print("start setup device")
    devConfig['profiles_id'] = None
    devpList = client.get_devicesProfilesList(org_id)
    for devp in devpList['result']:
        if devp['name'] == devConfig['profiles_name']:
            devConfig['profiles_id'] = devp['id']
            print("---> use dev_pf,", devConfig['profiles_name'], " id: ",
                  devConfig['profiles_id'])
            break
    if devConfig['profiles_id'] is None:
        devConfig['profiles_id'] = client.add_devicesProfiles(
            name=devConfig['profiles_name'], nsID=ns_id,
            loraWANVersion=devConfig['profiles_mac_version'],
            regRevion=devConfig["profiles_reg_version"],
            organID=org_id)

        print("---> add dev_pf,", devConfig['profiles_name'], " id: ",
              devConfig['profiles_id'])

    needAdd = True
    dev = client.get_devicesByEui(devConfig['eui'])
    if dev is not None:
        print("---> use dev,", devConfig['name'], " eui ",devConfig['eui'])
        needAdd = False

    if needAdd:
        client.add_devices(
            name=devConfig['name'],
            appID=app_id,
            devEUI=devConfig['eui'],
            devProfileID=devConfig['profiles_id'],
            skipFCntCheck=devConfig['skip_fcnt'])

        print("---> add dev,", devConfig['name'], " eui: ", devConfig['eui'])

    client.set_deviceActivate(
        devEui=devConfig['eui'], devAddr=devConfig['addr'],
        nsSessionEncKey=devConfig['network_session_encription_key'],
        nsSessionIntKey=devConfig['serving_network_session_integrity_key'],
        fnsSessionIntKey=devConfig['forwarding_network_session_integrity_key'],
        appKey=devConfig['application_session_key'])


def setup_devicegroup(client, devConfig, org_id, ns_id, app_id):
    print("start setup device group ", devConfig['name_prefix'])
    devConfig['profiles_id'] = None
    devpList = client.get_devicesProfilesList(org_id)
    for devp in devpList['result']:
        if devp['name'] == devConfig['profiles_name']:
            devConfig['profiles_id'] = devp['id']
            print("---> use dev_pf,", devConfig['profiles_name'], " id: ",
                  devConfig['profiles_id'])
            break
    if devConfig['profiles_id'] is None:
        devConfig['profiles_id'] = client.add_devicesProfiles(
            name=devConfig['profiles_name'], nsID=ns_id,
            loraWANVersion=devConfig['profiles_mac_version'],
            regRevion=devConfig["profiles_reg_version"],
            organID=org_id)

        print("---> add dev_pf,", devConfig['profiles_name'], " id: ",
              devConfig['profiles_id'])

    store_device_env = devConfig['name_prefix'] + "devlist.sh"
    with open(store_device_env,'wt') as f:
        # add devices
        for device_index in range(0, devConfig['dev_number']):
            # 1 -> "0001"
            deviceSuffix = struct.pack('>h', device_index)
            deviceSuffix = binascii.hexlify(deviceSuffix)

            devEui = devConfig['eui_prefix'] + deviceSuffix
            devName = devConfig['name_prefix'] + deviceSuffix
            devAddr = devConfig['addr_prefix'] + deviceSuffix

            needAdd = True
            dev = client.get_devicesByEui(devEui)
            if dev is not None:
                print("---> use dev,", devName, " eui ", devEui)
                needAdd = False

            if needAdd:
                client.add_devices(
                    name=devName,
                    appID=app_id,
                    devEUI=devEui,
                    devProfileID=devConfig['profiles_id'],
                    skipFCntCheck=devConfig['skip_fcnt'])
                print("---> add dev,", devName, " eui: ", devEui)

            """
            correct key len = 32, if you set len=28 in confile,
            here weill concat the suffix(len=4)
            otherwise, it will error when set device activate
            """
            nsSessionEncKey = devConfig['network_session_encription_key']
            if len(nsSessionEncKey) == 28:
                nsSessionEncKey += deviceSuffix

            nsSessionIntKey = devConfig['serving_network_session_integrity_key']
            if len(nsSessionIntKey) == 28:
                nsSessionIntKey += deviceSuffix

            fnsSessionIntKey = devConfig['forwarding_network_session_integrity_key']
            if len(fnsSessionIntKey) == 28:
                fnsSessionIntKey += deviceSuffix

            appKey = devConfig['application_session_key']
            if len(appKey) == 28:
                appKey += deviceSuffix

            client.set_deviceActivate(
                devEui=devEui, devAddr=devAddr,
                nsSessionEncKey=nsSessionEncKey,
                nsSessionIntKey=nsSessionIntKey,
                fnsSessionIntKey=fnsSessionIntKey,
                appKey=appKey)

        # write device env to file, start device sim by other script
        dev_env = "export " + '''\
UDP_SERVER={} \
GATEWAY_MAC={} \
DEVICE_EUI={} \
DEVICE_ADDRESS={} \
DEVICE_NETWORK_SESSION_ENCRIPTION_KEY={} \
DEVICE_SERVING_NETWORK_SESSION_INTEGRITY_KEY={} \
DEVICE_FORWARDING_NETWORK_SESSION_INTEGRITY_KEY={} \
DEVICE_APPLICATION={} \
DEVICE_APPLICATION_SESSION_KEY={} \
MQTT_SERVER={}\n'''.format(
                    devConfig['gateway_bridge_udp'], devConfig['gw_mac'],
                    devEui, devAddr,
                    nsSessionEncKey, nsSessionIntKey, fnsSessionIntKey,
                    app_id, appKey,
                    devConfig['app_mqtt']
                )
        f.write(dev_env)

def setup_app(client, appConfig, org_id):
    print("start setup app")

    # setup networkserver
    ns = appConfig['ns']
    setup_ns(client, ns, org_id)

    # setup gateways
    for gw in ns['gw']:
        setup_gw(client, gw, org_id, ns['id'])

    # setup application
    app = client.get_applications(org_id, search=appConfig['name'])
    if app['totalCount'] == '1':
        appConfig['id'] = app['result'][0]['id']
        print("---> use app,", appConfig['name'], " id: ", appConfig['id'])
    else:
        appConfig['id'] = client.add_applications(
            appConfig['name'], org_id, appConfig['ns']['profiles_id'])
        print("---> add app,", appConfig['name'], " id: ", appConfig['id'])

    # setup devices
    for dev in ns['device']:
        setup_device(client, dev, org_id, ns['id'], appConfig['id'])

    # setup devices
    for dev in ns['device_group']:
        setup_devicegroup(client, dev, org_id, ns['id'], appConfig['id'])


if __name__ == "__main__":

    print(" -------- Start")
    config = init_conf()

    client = lora_client.Client(config["server"]["host"],
                                config["server"]["user"],
                                config["server"]["passwd"],
                                config["server"]["proxy"],
                                config["server"]["proxy"])

    # create or get organizations
    org_id = get_organizationsID(config)

    for app in config["app"]:
        setup_app(client, app, org_id)

