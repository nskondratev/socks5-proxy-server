import fs from 'node:fs'
import path from 'node:path'

import Promise from 'bluebird'
import chalk from 'chalk'
import Table from 'cli-table'
import dotenv from 'dotenv'
import figlet from 'figlet'

import redis from '../services/redis.js'
import { REDIS } from '../services/constants.js'
import { dirname } from '../utils/dirname.js'

const figletPromise = Promise.promisify(figlet)

if (fs.existsSync(path.join(dirname(import.meta.url), '..', '.env'))) {
  dotenv.config()
}

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

const logo = await figletPromise('Proxy server users stats', { font: 'Standard' })

console.log(chalk.blueBright(logo))

try {
  let [dataUsage, lastLogin] = await Promise.all([
    redis.hGetAll(REDIS.DATA_USAGE_KEY),
    redis.hGetAll(REDIS.AUTH_DATE_KEY)
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
