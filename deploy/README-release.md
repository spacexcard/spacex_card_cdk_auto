# 发布包与一键更新

管理后台「一键无痕更新」依赖 GitHub Release 上的预编译包：

- `cdk-bundle-linux-amd64.tgz`（内含 `cdk-recharge` + `web/` + `VERSION`）
- 可选 `cdk-bundle-linux-amd64.tgz.sha256`

## 本地构建

```bash
./scripts/build-release-bundle.sh 1.2.3
```

## 发布到 GitHub

```bash
git tag v1.2.3 && git push origin v1.2.3
gh release create v1.2.3 cdk-bundle-linux-amd64.tgz cdk-bundle-linux-amd64.tgz.sha256 --generate-notes
```

## 启用 GitHub Actions（可选）

将 `deploy/github-release.workflow.yml` 复制为仓库内：

`.github/workflows/release.yml`

推送该文件需要 PAT 勾选 **workflow** 权限。之后 `git tag v*` 会自动构建并上传 bundle。
