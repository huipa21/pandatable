import fs from 'node:fs'
import net from 'node:net'

const file = process.env.PT_MAIL_SINK_FILE
if (!file || !/^\/data\/tmp\/pt-smtp-[a-z]+\.jsonl$/.test(file))
  throw new Error('invalid test sink path')
fs.writeFileSync(file, '', { mode: 0o600 })

net
  .createServer((socket) => {
    let pending = ''
    let data = null
    let recipient = ''
    socket.setEncoding('utf8')
    socket.write('220 test-sink ESMTP\r\n')
    socket.on('data', (chunk) => {
      pending += chunk
      let end
      while ((end = pending.indexOf('\n')) >= 0) {
        const line = pending.slice(0, end).replace(/\r$/, '')
        pending = pending.slice(end + 1)
        if (data !== null) {
          if (line === '.') {
            fs.appendFileSync(file, JSON.stringify({ recipient, data }) + '\n')
            data = null
            socket.write('250 queued\r\n')
          } else {
            data += line + '\n'
          }
        } else if (line.startsWith('EHLO ')) {
          socket.write('250-test-sink\r\n250-AUTH PLAIN\r\n250 HELP\r\n')
        } else if (line.startsWith('AUTH PLAIN ')) {
          const value = Buffer.from(line.slice(11), 'base64').toString('utf8')
          socket.write(
            value === '\0test-user\0test-password'
              ? '235 authenticated\r\n'
              : '535 invalid credentials\r\n',
          )
        } else if (line.startsWith('MAIL FROM:')) {
          recipient = ''
          socket.write('250 ok\r\n')
        } else if (line.startsWith('RCPT TO:')) {
          recipient = line.slice(8).replace(/^<|>$/g, '')
          socket.write('250 ok\r\n')
        } else if (line === 'DATA') {
          data = ''
          socket.write('354 send data\r\n')
        } else if (line === 'QUIT') {
          socket.end('221 bye\r\n')
        } else {
          socket.write('500 unexpected command\r\n')
        }
      }
    })
  })
  .listen(2525, '127.0.0.1', () => console.log('READY'))
