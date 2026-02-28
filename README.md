# Prometheus for fnnas

飞牛NAS应用商店的 Prometheus 监控系统安装包。

## 简介

本项目将 [Prometheus](https://github.com/prometheus/prometheus) 打包为飞牛NAS应用商店格式  
方便用户在飞牛NAS上一键安装和使用 Prometheus 时序数据库及监控系统。

## 功能特性

- 开箱即用的 Prometheus 监控服务
- 支持 Basic Auth 认证保护
- 数据持久化存储（默认保留180天）
- 支持 amd64 和 arm64 架构

## 安装要求

- 飞牛NAS 系统版本 >= 0.8.10
- 端口 9090 可用

## 数据存储

所有数据和配置文件存储在如下目录，X对应安装时存储空间序号
```
/volX/@appshare/prometheus/prometheus/
```

目录结构：
- `prometheus.yml` - Prometheus 配置文件
- `data/` - 时序数据存储目录
- `prometheus.log` - 运行日志

## 默认配置

| 配置项 | 默认值 |
|--------|--------|
| 服务端口 | 9090 |
| 抓取间隔 | 15s |
| 规则评估间隔 | 15s |
| 数据保留时间 | 180天 |

默认抓取目标：
- Prometheus 自身 (127.0.0.1:9090)
- Node Exporter (127.0.0.1:9100)

## 配置修改

编辑配置文件：
```
/volX/@appshare/prometheus/prometheus/prometheus.yml
```

修改后重启应用生效。

### 忘记密码

如忘记 Basic Auth 密码，可编辑 `prometheus.yml` 文件，修改或删除 `basic_auth_users` 配置项。

## 构建

本项目使用 GitHub Actions 自动构建，支持以下架构：
- linux/amd64
- linux/arm64

手动触发构建：在 GitHub 仓库的 Actions 页面手动运行 "Test Release" 工作流。

## 相关链接

- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Prometheus GitHub](https://github.com/prometheus/prometheus)
- [飞牛NAS](https://www.fnnas.com/)
