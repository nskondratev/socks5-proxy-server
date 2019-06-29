const Promise = require('bluebird')
const inquirer = require('inquirer')
const figlet = Promise.promisify(require('figlet'))
const chalk = require('chalk')

const fs = require('fs')
const path = require('path')

if (fs.existsSync(path.join(__dirname, '..', '.env'))) {
  require('dotenv').config()
}

const container = require('../services')

const redis = container.cradle.redis
const CONSTANTS = container.cradle.constants

const deleteUser = async ({username}) => {
    const userPassword = await redis.hgetAsync(CONSTANTS.REDIS.AUTH_USER_KEY, username)
    if (!userPassword) {
      throw new Error('User with provided username not found')
    }
    await redis.batch([
      ['hdel', CONSTANTS.REDIS.AUTH_USER_KEY, username],
    ]).execAsync()
  }

;(async () => {
  const logo = await figlet('Delete user', {font: 'Standard'})
  console.log(chalk.blueBright(logo))
  try {
    const answers = await inquirer.prompt([
      {
        type: 'input',
        name: 'username',
        message: 'Input username and press Enter:'
      }
    ])
    await deleteUser(answers)
    console.log(chalk.green('User successfully deleted.'))
    process.exit(0)
  } catch (err) {
    console.log(chalk.red(err))
    process.exit(1)
  }
})()

