try {
  process.chdir(__dirname)
} catch (err) {
  console.log('Failed to change cwd:', err)
  process.exit(1)
}

const Promise = require('bluebird')
const bcrypt = Promise.promisifyAll(require('bcrypt'))
const fs = require('fs')
const path = require('path')

if (fs.existsSync(path.join(__dirname, '.env'))) {
  require('dotenv').config()
}
const container = require('./services')

const redis = container.cradle.redis
const logger = container.cradle.logger.get()
const CONSTANTS = container.cradle.constants

logger.info(`Start simple socks5 proxy server. Version: ${require('./package.json').version}`)

const socksServerOptions = {}

if (parseInt(process.env.REQUIRE_AUTH) === 1) {
  socksServerOptions.authenticate = async (username, password, cb) => {
    const handleError = err => {
      logger.error(err)
      setImmediate(cb, err)
    }
    logger.debug(`User @${username} trying to authenticate proxy server...`)
    try {
      const userPassword = await redis.hgetAsync(CONSTANTS.REDIS.AUTH_USER_KEY, username)
      if (!userPassword) {
        return handleError(new Error('User not found'))
      }
      const isPasswordCorrect = await bcrypt.compareAsync(password, userPassword)
      if (isPasswordCorrect) {
        // Successfully authenticated
        return setImmediate(cb)
      } else {
        return handleError(new Error('Incorrect password'))
      }
    } catch (err) {
      handleError(err)
    }
  }
}

const server = require('simple-socks').createServer(socksServerOptions)

// Start listening
logger.info(`Start listening on port: ${process.env.APP_PORT}`)
server.listen(parseInt(process.env.APP_PORT))

server.on('handshake', function () {
  logger.debug('New client connection')
})

// When authentication succeeds
server.on('authenticate', async username => {
  logger.info(`User @${username} successfully authenticated!`)
  await redis.hsetAsync(CONSTANTS.REDIS.AUTH_DATE_KEY, username, (new Date()).toISOString())
})

// When authentication fails
server.on('authenticateError', (username, err) => logger.info(`User @${username} fails authentication:`, err))

// When a reqest arrives for a remote destination
server.on('proxyConnect', (info, destination) => {
  logger.debug(`Connected to remote server at ${info.host}:${info.port}`)
  // destination.on('data', function (data) {
  //   console.log(data.length)
  // })
})

server.on('proxyData', async ({data, user, kill}) => {
  try {
    logger.debug(`Increase user @${user.username} data usage by ${data.length}`)
    await redis.hincrbyAsync(CONSTANTS.REDIS.DATA_USAGE_KEY, user.username, data.length)
  } catch (err) {
    logger.error(err)
  }
})

// When an error occurs connecting to remote destination
server.on('proxyError', function (err) {
  logger.error('Unable to connect to remote server:', err)
})

// When a proxy connection ends
server.on('proxyEnd', function (response, args) {
  logger.debug(`socket closed with code ${response}, args: ${JSON.stringify(args)}`)
})
