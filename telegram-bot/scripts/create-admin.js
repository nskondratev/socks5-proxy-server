const fs = require('fs')
const path = require('path')

const Promise = require('bluebird')
const inquirer = require('inquirer')
const figlet = Promise.promisify(require('figlet'))
const chalk = require('chalk')

if (fs.existsSync(path.join(__dirname, '..', '.env'))) {
  require('dotenv').config()
}

const container = require('../services')

const createAdmin = async data => {
  return container.cradle.util.createAdmin(data.username)
}

const main = async () => {
  const logo = await figlet('Create admin', {font: 'Standard'})
  console.log(chalk.blueBright(logo))
  try {
    const answers = await inquirer.prompt([
      {
        type: 'input',
        name: 'username',
        message: 'Input admin username and press Enter:'
      }
    ])
    await createAdmin(answers)
    console.log(chalk.green('Admin successfully created.'))
    process.exit(0)
  } catch (err) {
    console.log(chalk.red(err))
    process.exit(1)
  }
}

main()
