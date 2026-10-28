# Eino 知识库手机端

Flutter 客户端（iOS / Android），对接本仓库 HTTP API：登录、配置服务端与默认知识库/目录、扫描/拍照录入文档、流式对话检索。

## 环境要求

- Flutter SDK ≥ 3.24（Dart ≥ 3.5）
- Xcode（iOS） / Android Studio 或 SDK（Android）
- 已启动本仓库后端（见根目录 [README.md](../README.md)）

## 快速开始

```bash
cd mobile
flutter pub get

# iOS 模拟器
flutter run -d ios

# Android 模拟器
flutter run -d android
```

### 服务端地址

| 运行环境 | 建议地址 |
|---------|---------|
| iOS 模拟器 | `http://127.0.0.1:8090` |
| Android 模拟器 | `http://10.0.2.2:8090` |
| 真机 | `http://<电脑局域网IP>:8090` |

首次打开登录页填写服务端地址、租户 ID、用户名与密码；登录后可在「设置」里改默认知识库/目录并探测 `/health`。

## 功能

1. **设置**：服务端地址、健康检查、默认知识库与目录、退出登录
2. **录入**：文档扫描（边缘检测多页）/ 拍照 / 相册 → `POST /api/v1/documents/import`（服务端 OCR）
3. **对话**：选择检索范围，SSE 流式问答 `POST /api/v1/chat/query`，展示引用来源

## 目录结构

```
lib/
  api/         # Dio REST + SSE 客户端、模型
  services/    # AppState（SharedPreferences + 鉴权）
  screens/     # 登录 / 对话 / 录入 / 设置
  widgets/     # 气泡、知识库选择器、来源卡片
  theme/       # 单强调色主题
```

## 权限说明

- **相机 / 相册**：扫描与拍照录入
- **明文 HTTP**：开发期允许访问局域网 API（`NSAppTransportSecurity` / `usesCleartextTraffic`）

生产环境请改用 HTTPS，并收紧 ATS / 网络安全配置。

## 与 Web 的差异（首期）

未包含：租户管理、用户开关、Agent 模式、语音 ASR/TTS、召回标注。管理类操作请继续使用 [web/](../web/)。
