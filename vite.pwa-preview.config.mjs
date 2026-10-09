// Isolated production-asset verification: deliberately no backend proxy.
export default {
  preview: { host: '127.0.0.1', strictPort: true, proxy: {} },
}
