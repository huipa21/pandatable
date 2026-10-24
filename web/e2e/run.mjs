// A private stdin pipe lets ptdev request graceful worker/browser shutdown even
// on Windows, where Go cannot deliver os.Interrupt to a child.
let cancelled = false
process.stdin.resume()
process.stdin.once('end', () => {
  cancelled = true
})
const interrupt = setInterval(() => {
  if (cancelled && process.listenerCount('SIGINT') > 0) {
    clearInterval(interrupt)
    process.emit('SIGINT')
  }
}, 50)
interrupt.unref()

await import('@playwright/test/cli')
