# 阿里云OSS证书自动化工具

阿里云OSS自定义域名证书自动更新工具。

## 原理

借助于 Let's Encrypt 证书，使用阿里云OSS对象存储API、阿里云证书管理服务API和阿里云CDN API实现阿里云OSS自定义域名证书的自动更新

![oss-auto-cert.png](oss-auto-cert.png)

## 使用

### 配置说明

- 完整yaml配置示例:

```yaml
# 企业微信机器人webhook地址
webhook: https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxxxxx-xxxxx-xxxxx-xxxxxx-xxxxxxx
# 证书申请配置
acme:
  email: 邮箱地址
  data-dir: 申请的证书文件保存目录（绝对路径）
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

- OSS_ACCESS_KEY_ID：阿里云accessKeyID，必须

- OSS_ACCESS_KEY_SECRET：阿里云accessKeySecret，必须


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

最新稳定版本容器镜像: `ghcr.io/nekoimi/oss-auto-cert:alpine`

```shell
# 运行例子:
docker run -d --rm -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml -e OSS_ACCESS_KEY_ID=xxx -e OSS_ACCESS_KEY_SECRET=xxx  ghcr.io/nekoimi/oss-auto-cert:alpine
```

也可以将申请的证书持久化，默认保存位置是：`/var/lib/oss-auto-cert`，优先使用配置文件中的`acme.data-dir`配置路径

```shell
# 持久化申请的证书文件
docker run -d --rm -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml -v $PWD/certs:/var/lib/oss-auto-cert -e OSS_ACCESS_KEY_ID=xxx -e OSS_ACCESS_KEY_SECRET=xxx ghcr.io/nekoimi/oss-auto-cert:alpine
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

## 感谢

- [go-acme/lego](https://github.com/go-acme/lego)
- [阿里云OpenAPI](https://api.aliyun.com)

## License

[LICENSE](LICENSE)
