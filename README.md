# 阿里云OSS证书自动化工具

阿里云OSS自定义域名证书自动更新工具。

## 原理

借助于 Let's Encrypt 证书，使用阿里云OSS对象存储API、阿里云证书管理服务API和阿里云CDN API实现阿里云OSS自定义域名证书的自动更新

![oss-auto-cert.png](oss-auto-cert.png)

## 使用

### 配置文件说明

完整配置示例:

```yaml

```

### 环境变量说明

- A

- B

- C

### Docker

```shell
docker run -dit -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml ghcr.io/nekoimi/oss-auto-cert:latest
```

### Systemd

推荐以服务方式管理

- 从 [`release`](releases) 下载稳定版

- 添加 `oss-auto-cert.service` 配置

```html

```

## 感谢

- [go-acme/lego](https://github.com/go-acme/lego)
- [阿里云OpenAPI](https://api.aliyun.com)

## License

[MIT](LICENSE)
