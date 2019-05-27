#!/usr/bin/env python
# coding=utf-8
# by orientlu

import types
import json
import requests
import httplib
import random

host = "http://192.168.234.128:8080"
user = "admin"
passwd = "admin"

class Client:
    """
    client operate lora restful api
    http:ip:port
    """

    def __init__(self, host, user="admin", passwd="admin", http_proxy=None, https_proxy=None):
        self.host = host
        self.user = user
        self.passwd = passwd

        self.proxies = {
            "http": http_proxy,
            "https": https_proxy,
        }

        self.jwt = self.get_jwt()

        self.headers = {
            'Accept': 'application/json',
            'Content-Type': 'application/json',
            'User-Agent': 'Mozilla/5.0',
            'Grpc-Metadata-Authorization': 'Bearer ' + self.jwt,
        }


    """
    get json web token
    """
    def get_jwt(self):
        data = {
            "username": self.user,
            "password": self.passwd
        }
        headers = {
            "Content-Type": "application/json",
            'User-Agent': 'Mozilla/5.0'
        }
        url = self.host + "/" + "api/internal/login"
        rsp = requests.post(url, json=data, proxies=self.proxies,
                            headers=headers)

        if rsp.status_code == httplib.OK:
            return json.loads(rsp.text)['jwt']
        else:
            print("Get jwt : ", rsp.text)
            return None

    def _post(self, url, data={}, headers=None):
        if headers is None:
            headers = self.headers
        rsp = requests.post(url, json=data, proxies=self.proxies, headers=headers)
        if rsp.status_code == httplib.OK:
            return json.loads(rsp.text)
        else:
            print(url, " post error: ", rsp.text)
            return None

    def _get(self, url, params={}, headers=None):
        if headers is None:
            headers = self.headers
        rsp = requests.get(url, params=params, proxies=self.proxies,
                           headers=headers)
        if rsp.status_code == httplib.OK:
            return json.loads(rsp.text)
        else:
            print(url, " get error: ", rsp.text)
            return None

    def _delete(self, url, params=None, headers=None):
        if headers is None:
            headers = self.headers
        rsp = requests.delete(url, params=params, proxies=self.proxies,
                           headers=headers)
        if rsp.status_code == httplib.OK:
            return json.loads(rsp.text)
        else:
            print(url, " delete error: ", rsp.text)
            return None

    """
    organizations
    """
    def get_organizationsList(self, search=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if search is not None:
            data["search"] = search

        url = self.host + "/api/organizations"
        return self._get(url, data)

    def get_organizations(self, id):
        url = self.host + "/api/organizations/" + str(id)
        return self._get(url)

    def create_organizations(self, name, displayname):
        data = {
         "organization": {
            "canHaveGateways": True,
            "displayName": displayname,
            "name": name
            }
        }
        url = self.host + "/api/organizations"
        rnt = self._post(url, data)
        if rnt is not None:
            return int(rnt["id"])
        else:
            return None

    def del_organizations(self, id):
        url = self.host + "/api/organizations/" + str(id)
        return self._delete(url)


    """
    networkServerService
    """
    def get_networkServersList(self, organizationID=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if organizationID is not None:
            data["organizationID"] = organizationID

        url = self.host + "/api/network-servers"
        return self._get(url, data)

    def add_networkServers(self, name, server):
        data = {
            "networkServer": {
                "name": name,
                "server": server,
                #"gatewayDiscoveryEnabled": False,
                #"caCert": "",
                #"gatewayDiscoveryDR": 0,
                #"gatewayDiscoveryInterval": 0,
                #"gatewayDiscoveryTXFrequency": 0,
                #"routingProfileCACert": "",
                #"routingProfileTLSCert": "",
                #"routingProfileTLSKey": "",
                #"tlsCert": "",
                #"tlsKey": ""
              }
        }
        url = self.host + "/api/network-servers"
        rnt = self._post(url, data)
        if rnt is not None:
            return int(rnt["id"])
        else:
            return None

    def del_networkServers(self, id):
        url = self.host + "/api/network-servers/" + str(id)
        return self._delete(url)

    """
    gateway
    """
    def get_gatewayProfilesList(self, nsID=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if nsID is not None:
            data["networkServerID"] = nsID

        url = self.host + "/api/gateway-profiles"
        return self._get(url, data)

    def add_gatewayProfiles(self, name, nsID="1", channels=[0, 1, 2]):
        data = {
            "gatewayProfile": {
                "channels": channels,
                "extraChannels": [
                  {
                    "bandwidth": 0,
                    "bitrate": 0,
                    "frequency": 0,
                    "modulation": "LORA",
                    "spreadingFactors": [
                      0
                    ]
                  }
                ],
                "name": name,
                "networkServerID": nsID,
            }
        }
        url = self.host + "/api/gateway-profiles"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt["id"]
        else:
            return None

    def del_gatewayProfiles(self, id):
        url = self.host + "/api/gateway-profiles/" + str(id)
        return self._delete(url)


    def get_gatewaysList(self, organizationID=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset
        }
        if organizationID is not None:
            data["organizationID"] = str(organizationID)

        url = self.host + "/api/gateways"
        return self._get(url, data)

    def add_gateways(self, name, gwID="20000000000000004", gwProfileID="1", nsID="1", organID="1", des="is a test gw"):
        data = {
            "gateway": {
               # "boards": [
               #   {
               #     "fineTimestampKey": "string",
               #     "fpgaID": "string"
               #   }
               # ],
                "description": des,
                "discoveryEnabled": True,
                "gatewayProfileID": gwProfileID,
                "id": gwID,
                "location": {
                  "accuracy": 2,
                  "altitude": 3,
                  "latitude": 3,
                  "longitude": 3,
                  "source": "UNKNOWN"
                },
                "name": name,
                "networkServerID": nsID,
                "organizationID": organID,
              }
        }
        url = self.host + "/api/gateways"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt
        else:
            return None

    def del_gateways(self, id):
        url = self.host + "/api/gateways/" + str(id)
        return self._delete(url)

    """
    service-profiles
    """
    def get_serviceProfilesList(self, organizationID=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if organizationID is not None:
            data["organizationID"] = organizationID

        url = self.host + "/api/service-profiles"
        return self._get(url, data)

    def add_serviceProfiles(self, name, nsID="1", organID="1", addGWMetaData=False, nwkGeoLoc=False):
        data = {
            "serviceProfile": {
                "addGWMetaData": addGWMetaData,
                "name": name,
                "networkServerID": nsID,
                "nwkGeoLoc": nwkGeoLoc,
                "organizationID": organID,
            }
        }
        url = self.host + "/api/service-profiles"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt["id"]
        else:
            return None

    def del_serviceProfiles(self, id):
        url = self.host + "/api/service-profiles" + str(id)
        return self._delete(url)


    """
    device-profiles
    """
    def get_devicesProfilesList(self, organizationID=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if organizationID is not None:
            data["organizationID"] = organizationID

        url = self.host + "/api/device-profiles"
        return self._get(url, data)

    def add_devicesProfiles(self, name, nsID="1", loraWANVersion="1.1.0",
                           regRevion='A', organID="1", fcnt=False,
                           classB=False, classC=False, join=False):
        data = {
            "deviceProfile": {
                "classBTimeout": 0,
                "classCTimeout": 0,
                "factoryPresetFreqs": [
                  0
                ],
                "macVersion": loraWANVersion,
                "maxDutyCycle": 0,
                "maxEIRP": 0,
                "name": name,
                "networkServerID": nsID,
                "organizationID": organID,
                "pingSlotDR": 0,
                "pingSlotFreq": 0,
                "pingSlotPeriod": 0,
                "regParamsRevision": regRevion,
                "rfRegion": "string",
                "rxDROffset1": 0,
                "rxDataRate2": 0,
                "rxDelay1": 0,
                "rxFreq2": 0,
                "supports32BitFCnt": fcnt,
                "supportsClassB": classB,
                "supportsClassC": classC,
                "supportsJoin": join,
            }
        }
        url = self.host + "/api/device-profiles"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt["id"]
        else:
            return None

    def del_devicesProfiles(self, id):
        url = self.host + "/api/device-profiles" + str(id)
        return self._delete(url)

    """
    multicast-groups
    """

    """
    application
    """
    def get_applications(self, organizationID=None, search=None, limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if organizationID is not None:
            data["organizationID"] = organizationID
        if organizationID is not None:
            data["search"] = search

        url = self.host + "/api/applications"
        return self._get(url, data)

    def add_applications(self, name, organID="1",
                         serviceProfileID="121c3530-f1b8-475a-beec-f780dae969fc",
                         des="add device test"):
        data = {
            "application": {
                "description": des,
                "name": name,
                "organizationID": organID,
                # "payloadCodec": plcodec,
                # "payloadDecoderScript": plDecodecScript,
                # "payloadEncoderScript": plEncodecScript,
                "serviceProfileID": serviceProfileID,
            }
        }
        url = self.host + "/api/applications"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt["id"]
        else:
            return None

    def del_applications(self, id):
        url = self.host + "/api/applications/" + str(id)
        return self._delete(url)

    """
    device
    """
    def get_devicesList(self, appID="1", limit=200, offset=0):
        data = {
            "limit": limit,
            "offset": offset,
        }
        if appID is not None:
            data["applicationID"] = appID

        url = self.host + "/api/devices"
        return self._get(url, data)

    def get_devicesByEui(self, Eui):
        url = self.host + "/api/devices/" + str(Eui)
        return self._get(url)


    def add_devices(self, name, appID="1", devEUI="0000000000000000",
                    devProfileID="09ea91ee-1b8b-4797-883b-16dfe466b3b0",
                    skipFCntCheck=True, des="add devices test"):
        data = {
            "device": {
                "applicationID": appID,
                "description": des,
                "devEUI": devEUI,
                "deviceProfileID": devProfileID,
                "name": name,
                "referenceAltitude": 0,
                "skipFCntCheck": skipFCntCheck,
            }
        }
        url = self.host + "/api/devices"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt
        else:
            return None

    def del_devices(self, id):
        url = self.host + "/api/devices/" + str(id)
        return self._delete(url)

    def set_deviceActivate(self, devEui, devAddr,
                           nsSessionEncKey, nsSessionIntKey, fnsSessionIntKey,
                           appKey):
        data = {
            "deviceActivation": {
                "devAddr": devAddr,
                "devEUI": devEui,
                "nwkSEncKey": nsSessionEncKey,
                "sNwkSIntKey": nsSessionIntKey,
                "fNwkSIntKey": fnsSessionIntKey,
                "appSKey": appKey,
                "fCntUp": 0,
                "nFCntDown": 0,
                "aFCntDown": 0,
            }
        }
        url = self.host + "/api/devices/" + str(devEui) + "/activate"
        rnt = self._post(url, data)
        if rnt is not None:
            return rnt
        else:
            return None


if __name__ == "__main__":
    client = Client(host, user, passwd)

    """
    ## example
    organization_name = "test_lora_" + str(random.randint(1,20000))

    for i in client.get_organizationsList()["result"]:
        if i["displayName"] == "test_lora":
            client.del_organizations(i["id"])

    oid = client.create_organizations(organization_name, "test_lora")
    print("new organization id :", oid)

    print client.get_organizationsList()["result"]

    print client.get_organizations(1)
    print client.get_networkServersList()
    print client.get_serviceProfilesList()
    """
    print client.get_gatewaysList()
