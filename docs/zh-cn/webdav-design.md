# WebDAV 性能设计决策

## 背景

早期尝试用原生 `x/net/webdav` 库桥接 SFTP（`webdav.FileSystem`）实现挂载，
在**高延迟链路**（如 200ms+ RTT 的远端）上出现严重性能问题：

- `walkFS` 在列目录时对**每个条目**额外调用一次 `fs.Stat`（忽略 `Readdir`
  已返回的属性），一次 PROPFIND 变成 **N 次远程往返**。
- 属性查找器（如 `getcontenttype`）对每个文件 `OpenFile` + 读文件头猜 MIME，
  同样是逐条目往返。
- 实测：210 项目录的 PROPFIND 耗时 **~50s**（每次往返 ~200ms × 210+）。

## 决策：自写 PROPFIND，一次 ReadDir 构建响应

QSSH 的 WebDAV 实现**不依赖 x/net/webdav**，自己写最小 RFC 4918 handler：

```
PROPFIND 目录 → 一次 sftp.ReadDir() → 用返回的 os.FileInfo（mode/size/
modtime/IsDir）直接构造 MultiStatus 响应
```

- **零逐条目往返**：一次 ReadDir 拿到全部条目属性，无需二次 Stat/Open。
- **getcontenttype 用扩展名猜测**（`mime.TypeByExtension`），不读文件。
- 实测：210 项目录 PROPFIND **~1.1s**（≈ 单次 SFTP 往返，已到协议极限）。

## 性能对比（213ms RTT 主机，/etc 210 项）

| 方案 | PROPFIND / ls -l 延迟 |
| --- | --- |
| 原生 x/net/webdav（逐条目 stat） | ~50s |
| 原生 SFTP 逐条目 stat（基线） | ~33s |
| sshfs (FUSE) | ~1.7s |
| **QSSH 自写 WebDAV（一次 ReadDir）** | **~1.1s** |

## 附加优化

- **并发写**（`ReadFromWithConcurrency`）：上传从 52s → 6.7s（10MB @ 213ms RTT），
  避免 `io.Copy` 退化为逐包等 ACK。
- 完整方案对比见 `archive/MOUNT_EXPERIMENTS_REPORT.md`。
