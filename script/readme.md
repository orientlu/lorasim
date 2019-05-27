
## lora app restful api 封装，lora_client.py

## 环境准备
prepare.py读取配置文件 config.toml，调用 lora_client.py 向指定环境配置组织，sever，gw和应用节点信息等参数

# 导入节点组
方便测试，提供成批导入虚拟节点，见配置文件  [[app.ns.device_group]]
实现单个仿真网关带一组节点，单个组设备最大值为 65535
配置导入节点组后，执行导入后生成节点组绑定gw的配置文件 gw-{gw-eui}-config.toml 供loracli 读取启动对应的仿真 gw

```bash
./start_device.sh
#启动仿真gw，pid 写入 gw-{gw-eui}-config.toml.pid
./stop_device.sh
#停止启动的仿真gw
```


## api test
```bash
curl -X POST 'http://192.168.234.128:8080/api/internal/login'  -H "Content-Type: application/json" -d '{"username":"admin", "password":"admin"}' -x ""


curl -X GET --header 'Accept: application/json' -H 'Grpc-Metadata-Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJsb3JhLWFwcC1zZXJ2ZXIiLCJleHAiOjE1NTc5NzgwMTUsImlzcyI6ImxvcmEtYXBwLXNlcnZlciIsIm5iZiI6MTU1Nzg5MTYxNSwic3ViIjoidXNlciIsInVzZXJuYW1lIjoiYWRtaW4ifQ.-ExzgAgSzj3PdlW-4U7pt38EUXUIModPFMmebEosWBY' 'http://192.168.234.128:8080/api/organizations?limit=10' -x ""


curl -X GET --header 'Accept: application/json' -H 'Grpc-Metadata-Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJsb3JhLWFwcC1zZXJ2ZXIiLCJleHAiOjE1NTgwNjk0MTUsImlzcyI6ImxvcmEtYXBwLXNlcnZlciIsIm5iZiI6MTU1Nzk4MzAxNSwic3ViIjoidXNlciIsInVzZXJuYW1lIjoiYWRtaW4ifQ.pH3xguBcH6ybwIeNiCv4SuhasbvwaPmJRz5LPDGxgw4' 'http://192.168.234.128:8080/api/gateways?limit=10' -x ""
```
