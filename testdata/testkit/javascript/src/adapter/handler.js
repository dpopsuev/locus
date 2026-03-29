const express = require('express');
const { Service } = require('../domain/entity');

function createRouter(service) {
  const router = express.Router();

  router.get('/entities/:id', (req, res) => {
    const entity = service.getEntity(req.params.id);
    if (!entity) return res.status(404).json({ error: 'not found' });
    res.json(entity);
  });

  return router;
}

module.exports = { createRouter };
