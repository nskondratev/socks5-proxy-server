import fs from 'node:fs'
import path from 'node:path'

import dotenv from 'dotenv'
import Promise from 'bluebird'
import inquirer from 'inquirer'
import figlet from 'figlet'
import chalk from 'chalk'

import { client as redis, Store } from '../services/redis.js'
import { dirname } from '../services/utils.js'

const figletPromise = Promise.promisify(figlet)

if (fs.existsSync(path.join(dirname(import.meta.url), '..', '.env'))) {
  dotenv.config()
}

const store = new Store(redis)

const logo = await figletPromise('Delete admin', { font: 'Standard' })

console.log(chalk.blueBright(logo))

try {
  const answers = await inquirer.prompt([
    {
      type: 'input',
      name: 'username',
      message: 'Input admin username and press Enter:'
    }
  ])

  await store.deleteAdmin(answers.username)

  console.log(chalk.green('Admin successfully deleted.'))
  process.exit(0)
} catch (err) {
  console.log(chalk.red(err))
  process.exit(1)
}
