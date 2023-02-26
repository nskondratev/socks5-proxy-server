const Promise = require('bluebird')
const figlet = Promise.promisify(require('figlet'))
const chalk = require('chalk')
const Table = require('cli-table')

const fs = require('fs')
const path = require('path')

if (fs.existsSync(path.join(__dirname, '..', '.env'))) {
  require('dotenv').config()
}

const container = require('../services')

const redis = container.cradle.redis
const CONSTANTS = container.cradle.constants

const parseStatsResults = (data, lastLoginData) => {
  const users = []
  if (data) {
    Object.keys(data).forEach(username => users.push({ username, usage: parseInt(data[username]) }))
    users.sort((a, b) => b.usage - a.usage)
    return users.map((u, i) => {
      let usage = `${u.usage} B`
      if (u.usage > 1073741824) {
        // Usage in GB
        usage = `${(u.usage / 1073741824).toFixed(2)} GB`
      } else if (u.usage > 1048576) {
        // Usage in MB
        usage = `${(u.usage / 1048576).toFixed(2)} MB`
      } else if (u.usage > 1024) {
        // Usage in KB
        usage = `${(u.usage / 1024).toFixed(2)} KB`
      }
      return [i + 1, ...Object.values(u), usage, lastLoginData[u.username] || '-']
    })
  }
  return users
}

;(async () => {
  await redis.connect()

  const logo = await figlet('Proxy server users stats', { font: 'Standard' })
  console.log(chalk.blueBright(logo))
  try {
    let [dataUsage, lastLogin] = await Promise.all([
      redis.hGetAll(CONSTANTS.REDIS.DATA_USAGE_KEY),
      redis.hGetAll(CONSTANTS.REDIS.AUTH_DATE_KEY)
    ])
    dataUsage = parseStatsResults(dataUsage, lastLogin)
    const table = new Table({
      head: ['#', 'Username', 'Data usage (in bytes)', 'Data usage (human readable)', 'Last login'],
      colWidths: [4, 30, 30, 30, 30]
    })
    table.push(...dataUsage)
    console.log(table.toString())
    process.exit(0)
  } catch (err) {
    console.log(chalk.red(err))
    process.exit(1)
  }
})()
