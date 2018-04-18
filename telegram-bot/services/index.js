const awilix = require('awilix')
const path = require('path')
const _ = require('lodash')

const container = awilix.createContainer()

container.loadModules([
  ['services/*/index.js', awilix.Lifetime.SINGLETON]
], {
  formatName: (name, descriptor) => {
    let basename = path.basename(descriptor.path)
    if (basename.startsWith('index.')) {
      basename = path.basename(path.dirname(descriptor.path))
    }
    basename = basename.replace(/(\.singleton)?.js/i, '')
    return _.camelCase(basename)
  }
})

module.exports = container
