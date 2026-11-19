/* Read-only export. No document mutation or network requests. */
figma.showUI(__html__, { width: 440, height: 380 });
let running = false;
figma.ui.onmessage = async (message) => {
  if (message.type !== 'export' || running) return;
  running = true;
  const failures = [],
    index = [],
    imageHashes = new Set();
  const send = (file, data) => figma.ui.postMessage({ type: 'file', file, data });
  const safe = (value) => value.replace(/[\\/:*?"<>|\x00-\x1f]/g, '-').slice(0, 100);
  const collectImages = (node) => {
    for (const key of ['fills', 'strokes']) {
      const paints = node[key];
      if (Array.isArray(paints))
        for (const paint of paints)
          if (paint.type === 'IMAGE' && paint.imageHash) imageHashes.add(paint.imageHash);
    }
    if ('children' in node) node.children.forEach(collectImages);
  };
  const rootsIn = (parent) =>
    parent.children.flatMap((node) =>
      node.type === 'SECTION'
        ? rootsIn(node)
        : ['FRAME', 'COMPONENT', 'COMPONENT_SET', 'INSTANCE', 'GROUP'].includes(node.type) &&
            node.visible !== false
          ? [node]
          : [],
    );
  try {
    await figma.loadAllPagesAsync();
    const pages = message.scope === 'current' ? [figma.currentPage] : [...figma.root.children];
    const roots = pages.flatMap((page) => rootsIn(page).map((node) => ({ page, node })));
    if (!roots.length) throw new Error('На выбранных страницах нет фреймов для экспорта.');
    let done = 0;
    for (const { page, node } of roots) {
      const base = safe(page.name) + '/' + safe(node.id) + '-' + safe(node.name);
      figma.ui.postMessage({
        type: 'progress',
        text: `${++done} / ${roots.length} · ${node.name}`,
      });
      collectImages(node);
      for (const format of ['JSON_REST_V1', 'SVG', 'PNG']) {
        try {
          const options =
            format === 'SVG'
              ? { format, svgOutlineText: false, svgIdAttribute: true }
              : format === 'PNG'
                ? { format, constraint: { type: 'SCALE', value: 1 } }
                : { format };
          const data = await node.exportAsync(options);
          send(
            base + '.' + (format === 'JSON_REST_V1' ? 'json' : format.toLowerCase()),
            format === 'JSON_REST_V1' ? JSON.stringify(data, null, 2) : Array.from(data),
          );
        } catch (error) {
          failures.push({ node: node.id, name: node.name, format, error: String(error) });
        }
      }
      index.push({
        id: node.id,
        name: node.name,
        page: page.name,
        width: node.width,
        height: node.height,
        path: base,
      });
    }
    for (const hash of imageHashes) {
      try {
        const image = figma.getImageByHash(hash);
        if (!image) throw new Error('Image unavailable');
        send('assets/' + hash + '.bin', Array.from(await image.getBytesAsync()));
      } catch (error) {
        failures.push({ image: hash, error: String(error) });
      }
    }
    try {
      const variables = await figma.variables.getLocalVariablesAsync();
      const collections = await figma.variables.getLocalVariableCollectionsAsync();
      send(
        'variables.json',
        JSON.stringify(
          {
            variables: variables.map((v) => ({
              id: v.id,
              name: v.name,
              resolvedType: v.resolvedType,
              collection: v.variableCollectionId,
              valuesByMode: v.valuesByMode,
            })),
            collections: collections.map((c) => ({
              id: c.id,
              name: c.name,
              defaultModeId: c.defaultModeId,
              modes: c.modes,
            })),
          },
          null,
          2,
        ),
      );
    } catch (error) {
      failures.push({ format: 'variables', error: String(error) });
    }
    send(
      'index.json',
      JSON.stringify(
        {
          fileName: figma.root.name,
          exportedAt: new Date().toISOString(),
          screens: index,
          failures,
        },
        null,
        2,
      ),
    );
    figma.ui.postMessage({ type: 'done', count: index.length, failures: failures.length });
  } catch (error) {
    figma.ui.postMessage({ type: 'error', text: String(error) });
  } finally {
    running = false;
  }
};
