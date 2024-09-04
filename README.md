# 阿里云OSS证书自动化工具

阿里云OSS自定义域名证书自动更新工具。

## 原理

借助于 Let's Encrypt 证书，使用阿里云OSS对象存储API、阿里云证书管理服务API和阿里云CDN API实现阿里云OSS自定义域名证书的自动更新

![oss-auto-cert.png](oss-auto-cert.png)

## 使用

### 配置说明

- 完整yaml配置示例:

```yaml
# 消息通知webhook地址
webhook: https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxxxxx-xxxxx-xxxxx-xxxxxx-xxxxxxx
# 消息通知内容go模版，将模版内容渲染后，作为POST请求的body内容发送到`webhook`地址
# 模版唯一变量：`Message` - 消息内容
# 不配置默认使用企业微信机器人的消息模版，如下：
### 企业微信机器人文本消息示例配置：
### 参考文档：https://developer.work.weixin.qq.com/document/path/91770#%E6%96%87%E6%9C%AC%E7%B1%BB%E5%9E%8B
webhook-tpl: |
  {
    "msgtype": "text",
    "text": {
      "content": "{{ .Message }}"
    }
  }

#### 钉钉机器人文本消息示例配置：
#### 参考文档：https://open.dingtalk.com/document/orgapp/custom-robot-access#title-nfv-794-g71
#webhook-tpl: |
#  {
#    "msgtype": "text",
#    "text": {
#      "content":"{{ .Message }}"
#    }
#  }

# 证书申请配置
acme:
  email: 申请证书邮箱地址（可以收到域名证书相关通知）
  data-dir: 申请的证书文件保存目录（绝对路径）
  expired-early: 30 # 证书提前过期时间，单位：天（默认提前15天过期）
# 阿里云OSS配置 Bucket信息列表
buckets:
  - name: bucket名称1
    endpoint: bucket信息endpoint地址

  - name: bucket名称2
    endpoint: bucket信息endpoint地址

  - name: bucket名称3
    endpoint: bucket信息endpoint地址
```

##### 环境变量配置说明

环境变量配置优先级最高，如果同时配置了环境变量和yaml，将以环境变量为准。

- OSS_ACCESS_KEY_ID：阿里云accessKeyID，必须，只能通过环境变量设置

- OSS_ACCESS_KEY_SECRET：阿里云accessKeySecret，必须，只能通过环境变量设置

- ACME_EMAIL：申请证书邮箱地址（可以收到域名证书相关通知）

- ACME_DATA_DIR：申请的证书文件保存目录（绝对路径）

- ACME_EXPIRED_EARLY：证书提前过期时间，单位：天（默认提前15天过期）

- DEBUG：调试模式，默认关闭
    - 证书检测会直接为过期
    - true - 开启调试，使用`LEDirectoryStaging`环境
    - false - 关闭调试，使用`LEDirectoryProduction`环境

##### 运行参数配置说明

- log-level：日志级别，默认info级别

```shell
# 例子：
./oss-auto-cert -log-level=warn
```

- config：配置文件路径，指定yaml配置文件路径

```shell
# 例子：
./oss-auto-cert -config=/home/user/oss-auto-cert/config.yaml
```

### Docker部署（推荐）

最新稳定版本容器镜像:

dockerhub: `nekoimi/oss-auto-cert:latest`

ghcr.io: `ghcr.io/nekoimi/oss-auto-cert:latest`

```shell
# 运行例子:
docker run -d --rm -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml -e OSS_ACCESS_KEY_ID=xxx -e OSS_ACCESS_KEY_SECRET=xxx  ghcr.io/nekoimi/oss-auto-cert:alpine
```

也可以将申请的证书持久化，默认保存位置是：`/var/lib/oss-auto-cert`，优先使用配置文件中的`acme.data-dir`配置路径

```shell
# 持久化申请的证书文件
docker run -d --rm -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml -v $PWD/certs:/var/lib/oss-auto-cert -e OSS_ACCESS_KEY_ID=xxx -e OSS_ACCESS_KEY_SECRET=xxx ghcr.io/nekoimi/oss-auto-cert:alpine
```

- docker-compose 配置

```yaml
version: "3.8"
services:
  oss-auto-cert:
    image: ghcr.io/nekoimi/oss-auto-cert:alpine
    container_name: oss-auto-cert
    hostname: oss-auto-cert
    network_mode: host
    command:
      - -log-level=warn
    volumes:
      - $PWD/config.yaml:/etc/oss-auto-cert/config.yaml
      - $PWD/certs:/var/lib/oss-auto-cert
    privileged: true
    restart: unless-stopped
    environment:
      OSS_ACCESS_KEY_ID: xxx
      OSS_ACCESS_KEY_SECRET: xxx
      ACME_EMAIL: xxxxx@xxxxxx.com
      ACME_DATA_DIR: /data
      ACME_EXPIRED_EARLY: 15

```

### Systemd部署

- 下载最新版本：[`release`](https://github.com/nekoimi/oss-auto-cert/releases)

- 创建配置文件：`/etc/oss-auto-cert/config.yaml`

- 创建service配置: `oss-auto-cert.service`，保存路径：`/usr/lib/systemd/system`

```html
[Unit]
Description=阿里云OSS证书自动化工具
Documentation=https://github.com/nekoimi/oss-auto-cert
After=network.target local-fs.target

[Service]
Type=simple
ExecStart=/usr/bin/oss-auto-cert > /var/log/oss-auto-cert.log 2>&1
ExecStop=kill -9 $(pidof oss-auto-cert)
ExecReload=kill -9 $(pidof oss-auto-cert) && /usr/bin/oss-auto-cert > /var/log/oss-auto-cert.log 2>&1
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

- 开启

```shell
systemctl enable oss-auto-cert

systemctl start oss-auto-cert
```

- 停止

```shell
systemctl stop oss-auto-cert
```

## 感谢

- [go-acme/lego](https://github.com/go-acme/lego)
- [阿里云OpenAPI](https://api.aliyun.com)

## License

[LICENSE](LICENSE)
