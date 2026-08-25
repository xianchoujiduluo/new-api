# GitHub Actions 发布邮件通知

## 当前行为

所有发布 Workflow 都包含独立的 `notify` job。通知通过 `if: always()` 等待该
Workflow 的全部发布 job，无论发布成功、失败或部分任务被跳过，都会尝试发送结果邮件。

通知覆盖以下发布流程：

- Docker 镜像及多架构清单发布
- Linux、macOS、Windows 和 Electron Release
- 前端 `latestFront` Release
- 后端 `latestBackend` Release（含签名清单和 Linux `amd64`/`arm64` 二进制）
- GitCode Release 同步

`ci.yml` 和 `pr-check.yml` 不产生发布产物，因此不发送发布邮件。

## 配置项

在仓库或组织的 **Settings → Secrets and variables → Actions** 中配置。多个仓库共享时，推荐配置在组织级别，并将仓库加入可访问范围。

### Actions Variables

| Name | Example | Description |
| --- | --- | --- |
| `SMTP_HOST` | `smtp.gmail.com` | SMTP server address |
| `SMTP_PORT` | `465` | SMTP port; the current action uses implicit TLS |

### Actions Secrets

| Name | Description |
| --- | --- |
| `SMTP_USERNAME` | SMTP login, normally the complete Gmail address |
| `SMTP_PASSWORD` | SMTP password or provider app password |
| `SMTP_FROM` | Sender address, normally the authenticated Gmail address |
| `NOTIFY_EMAIL_TO` | Recipient address; multiple addresses may be comma-separated if supported by the action |
| `BACKEND_UPDATE_SIGNING_KEY` | Ed25519 私钥（PKCS#8 PEM 或 Base64），仅供 `latestBackend` 清单签名使用 |

`GITHUB_TOKEN` is automatically provided by GitHub Actions and does not need to be created manually.

For Gmail, enable 2-Step Verification and create a Google App Password. Use that 16-character app password as `SMTP_PASSWORD`; do not use the normal Google account password.

## Workflow 配置

在需要通知的 workflow 中添加一个通知 job。`needs` 必须列出该 workflow 的所有发布 job，`if: always()` 确保前置 job 失败时仍尝试发送结果：

```yaml
notify:
  name: Email workflow result
  needs: [publish]
  if: always()
  runs-on: ubuntu-latest
  steps:
    - name: Send workflow result email
      uses: dawidd6/action-send-mail@v6
      with:
        server_address: ${{ vars.SMTP_HOST }}
        server_port: ${{ vars.SMTP_PORT }}
        secure: true
        username: ${{ secrets.SMTP_USERNAME }}
        password: ${{ secrets.SMTP_PASSWORD }}
        from: ${{ secrets.SMTP_FROM }}
        to: ${{ secrets.NOTIFY_EMAIL_TO }}
        subject: "[${{ github.repository }}] workflow result"
        body: |
          Repository: ${{ github.repository }}
          Ref: ${{ github.ref_name }}
          Commit: ${{ github.sha }}
          Result: ${{ needs.publish.result }}
```

对于本项目的前端发布，通知位于 `.github/workflows/frontend-release.yml`，并等待
`release` job。仅配置 Variables 和 Secrets 不会自动发送邮件；新增发布 Workflow 时，
仍须添加自己的通知 job，并在 `needs` 中列出全部发布 job。

后端独立发布位于 `.github/workflows/backend-release.yml`。其签名私钥只保存在
`BACKEND_UPDATE_SIGNING_KEY` Secret 中；运行时镜像内置对应公钥，禁止把私钥写入仓库、
镜像或系统设置。只有需要切换信任根时，才通过容器环境变量
`BACKEND_UPDATE_PUBLIC_KEY` 覆盖内置公钥。

## 验证

推送或手动运行 workflow 后，在 **Actions** 页面确认 `Email workflow result` job 已执行。失败时重点检查 SMTP host/port、App Password、`from` 地址以及组织 Secrets 对当前仓库的访问范围。
