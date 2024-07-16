# 阿里云OSS证书自动化工具

阿里云OSS自定义域名证书自动更新工具。

## 原理

借助于 Let's Encrypt 证书，使用阿里云OSS对象存储API、阿里云证书管理服务API和阿里云CDN API实现阿里云OSS自定义域名证书的自动更新

![oss-auto-cert.png](oss-auto-cert.png)

## 使用

### 配置说明

- 完整yaml配置示例:

```yaml

```

- 环境变量配置说明

```html
A
B
C
```

### Docker部署（推荐）

最新稳定版本容器镜像: `ghcr.io/nekoimi/oss-auto-cert:latest`

```shell
# 运行例子:
docker run -d -v $PWD/config.yaml:/etc/oss-auto-cert/config.yaml ghcr.io/nekoimi/oss-auto-cert:latest
```

### Systemd部署

- 从 [`release`](releases) 下载稳定版可执行文件

- 添加 `oss-auto-cert.service` 配置

```html

```

## 感谢

- [go-acme/lego](https://github.com/go-acme/lego)
- [阿里云OpenAPI](https://api.aliyun.com)

## License

[MIT](LICENSE)
