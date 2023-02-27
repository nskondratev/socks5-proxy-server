import { existsSync } from 'node:fs'
import { join } from 'node:path'

import { compare } from 'bcrypt'
import dotenv from 'dotenv'
import { scheduleJob } from 'node-schedule'
import simpleSocks from 'simple-socks'

import packageJSON from './package.json' assert { type: 'json' }
import { dirname } from './utils/dirname.js'
import { REDIS } from './services/constants.js'
import logger from './services/logger.js'
import redis from './services/redis.js'

if (existsSync(join(dirname(import.meta.url), '.env'))) {
  dotenv.config()
}

logger.info(`Start simple socks5 proxy server. Version: ${packageJSON.version}`)

const socksServerOptions = {}

if (parseInt(process.env.REQUIRE_AUTH) === 1) {
  socksServerOptions.authenticate = async (username, password, cb) => {
    const handleError = err => {
      logger.error(err)
      setImmediate(cb, err)
    }

    logger.debug(`User @${username} trying to authenticate proxy server...`)

    try {
      const userPassword = await redis.hGet(REDIS.AUTH_USER_KEY, username)
      if (!userPassword) {
        return handleError(new Error('User not found'))
      }

      const isPasswordCorrect = await compare(password, userPassword)
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

const server = simpleSocks.createServer(socksServerOptions)

// Start listening
logger.info(`Start listening on port: ${process.env.APP_PORT}`)
server.listen(parseInt(process.env.APP_PORT))

server.on('handshake', () => {
  logger.debug('New client connection')
})

// When authentication succeeds
server.on('authenticate', async username => {
  logger.info(`User @${username} successfully authenticated!`)
  await redis.hSet(REDIS.AUTH_DATE_KEY, username, (new Date()).toISOString())
})

// When authentication fails
server.on('authenticateError', (username, err) => logger.info(`User @${username} fails authentication:`, err))

// When a request arrives for a remote destination
server.on('proxyConnect', info => logger.debug(`Connected to remote server at ${info.host}:${info.port}`))

server.on('proxyData', async ({ data, user }) => {
  try {
    if (user) {
      logger.debug(`Increase user @${user.username} data usage by ${data.length}`)
      await redis.hIncrBy(REDIS.DATA_USAGE_KEY, user.username, data.length)
    }
  } catch (err) {
    logger.error(err)
  }
})

// When an error occurs connecting to remote destination
server.on('proxyError', err => logger.error('Unable to connect to remote server:', err))

// When a proxy connection ends
server.on('proxyEnd', (response, args) => {
  logger.debug(`socket closed with code ${response}, args: ${JSON.stringify(args)}`)
})

// Clear data usage stats at the beginning of each month
scheduleJob('0 0 0 1 * *', async () => {
  logger.debug('Clear data usage...')
  try {
    await redis.del(REDIS.DATA_USAGE_KEY)
    logger.info('Data usage is cleared.')
  } catch (err) {
    logger.warn(err)
  }
})
