const path = require('path')
const fs = require('fs')

const MODELS_DIR = path.join(__dirname, '..', '..', 'models')

module.exports = container => {
  const logger = container.logger.get()
  logger.debug('Going to load models...')
  const models = {}
  fs
    .readdirSync(MODELS_DIR)
    .filter(filename => filename.endsWith('.js'))
    .forEach(filename => {
      const model = require(path.join(MODELS_DIR, filename))(container)
      models[model.name] = model
    })
  // Associate models
  Object.keys(models).forEach(modelName => {
    if (models[modelName].associate) {
      models[modelName].associate(models)
    }
  })
  logger.debug(`Loaded models: ${Object.keys(models).join(', ')}`)
  return models
}
