const path = require('path')
const log4js = require('log4js')

module.exports = () => ({
  get: (category = 'default') => {
    const logger = log4js.getLogger(category)

    const layout = {
      console: {
        type: 'pattern',
        pattern: '%[[%z] [%d] [%5.10p] - %]%m'
      },
      file: {
        type: 'pattern',
        pattern: '[%z] [%d] [%5.10p] - %m'
      }
    }

    log4js.configure({
      appenders: {
        console: {type: 'console', layout: layout.console},
        file: {
          type: 'file',
          filename: path.join(__dirname, '..', '..', 'logs', 'cheese.log'),
          maxLogSize: 52428800,
          backups: 10,
          keepFileExt: true,
          compress: true,
          layout: layout.file
        }
      },
      categories: {
        default: {appenders: ['console', 'file'], level: process.env.LOG_LEVEL || 'info'}
      }
    })
    return logger
  },
  getConnectMiddleware (level = 'info') {
    return log4js.connectLogger(this.get(), {level})
  },
  shutdown: () => log4js.shutdown()
})
