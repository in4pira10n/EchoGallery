/* Mobile record window: retain order in memory, page full records to local IndexedDB. */
(function (root) {
  'use strict';
  const indexFields = new Set(['id', 'uuid', 'media_kind', 'taken_at', 'uploaded_at',
    'is_favorite', 'is_super_favorite', 'deleted_at', 'deleted_by', 'root_debug', 'original_name', 'duration_ms']);
  function localStore(win) {
    const session = `${Date.now()}:${Math.random()}`;
    let serial = 0, opening, closed = false;
    function database() {
      if (!opening) opening = new Promise((resolve, reject) => {
        if (!win.indexedDB) { reject(new Error('IndexedDB unavailable')); return; }
        const request = win.indexedDB.open('echogallery-photo-window', 1);
        request.onupgradeneeded = () => {
          const store = request.result.createObjectStore('records');
          store.createIndex('saved', 'saved');
        };
        request.onerror = () => reject(request.error);
        request.onblocked = () => reject(new Error('Record window blocked'));
        request.onsuccess = () => {
          const db = request.result;
          db.onversionchange = () => db.close();
          // Only reap a crashed tab after obtaining its lifetime lock. A live
          // (even backgrounded) tab must never lose its archived records.
          if (win.navigator?.locks) {
            const tx = db.transaction('records', 'readonly');
            const sessions = new Set();
            const cursor = tx.objectStore('records').index('saved').openCursor(win.IDBKeyRange.upperBound(Date.now() - 86400000));
            cursor.onsuccess = () => { const row = cursor.result; if (row) { sessions.add(row.primaryKey[0]); row.continue(); } };
            tx.oncomplete = () => sessions.forEach(owner => {
              if (owner === session) return;
              void win.navigator.locks.request(`eg-photo-window:${owner}`, { ifAvailable: true }, lock => {
                if (!lock || closed) return;
                return new Promise(resolve => {
                  const cleanup = db.transaction('records', 'readwrite');
                  cleanup.objectStore('records').delete(win.IDBKeyRange.bound([owner, 0], [owner, Infinity]));
                  cleanup.oncomplete = cleanup.onabort = resolve;
                });
              }).catch(() => {});
            });
          }
          if (closed) { db.close(); reject(new Error('Record window closed')); }
          else resolve(db);
        };
      });
      return opening;
    }
    let unlock;
    const locked = win.navigator?.locks ? new Promise(resolve => {
      void win.navigator.locks.request(`eg-photo-window:${session}`, () => {
        resolve(); return new Promise(release => { unlock = release; });
      }).catch(resolve);
    }) : Promise.resolve();
    async function transaction(mode, work) {
      await locked;
      const db = await database();
      if (closed) throw new Error('Record window closed');
      return new Promise((resolve, reject) => {
        const tx = db.transaction('records', mode);
        let result;
        tx.oncomplete = () => resolve(result);
        tx.onerror = tx.onabort = () => reject(tx.error || new Error('Record window transaction failed'));
        result = work(tx.objectStore('records'));
      });
    }
    return {
      key: () => [session, ++serial],
      write: rows => transaction('readwrite', store => {
        for (const row of rows) store.put({ value: row.value, saved: Date.now() }, row.key);
      }),
      read: keys => transaction('readonly', store => {
        const result = new Array(keys.length);
        keys.forEach((key, index) => { store.get(key).onsuccess = event => { result[index] = event.target.result?.value; }; });
        return result;
      }),
      async dispose() {
        if (closed) return;
        try {
          if (opening) {
            await transaction('readwrite', store => store.delete(win.IDBKeyRange.bound([session, 0], [session, Infinity])));
            (await opening).close();
          }
        } catch (_) {} finally { closed = true; unlock?.(); }
      },
    };
  }
  function create({ window: win = root, store = localStore(win), onChange = () => {}, limit = 160 } = {}) {
    const records = new WeakMap(), resident = new Map();
    let pinned = new Set(), pending = null, timer = 0, disabled = false, disposed = false;
    let archived = 0, reads = 0;
    const holds = new Map();
    const protectedItem = item => pinned.has(item) || holds.has(item);
    const idle = callback => win.setTimeout(callback, 32);
    function track(items) {
      for (const item of items) {
        if (!item || records.has(item)) continue;
        const record = { key: store.key(), ref: new WeakRef(item), archived: false, loading: null };
        records.set(item, record); resident.set(record, record.ref);
      }
    }
    function schedule() {
      if (!disposed && !disabled && !timer && !pending && resident.size > Math.max(limit, pinned.size)) {
        timer = idle(() => { timer = 0; void trim(); });
      }
    }
    async function trim() {
      if (disposed || disabled || pending || resident.size <= Math.max(limit, pinned.size)) return;
      const rows = [];
      for (const [record, ref] of resident) {
        const item = ref.deref();
        if (!item) { resident.delete(record); continue; }
        if (!protectedItem(item)) rows.push({ item, record, key: record.key,
          value: structuredClone(item), fingerprint: JSON.stringify(item) });
        if (rows.length >= Math.min(64, resident.size - Math.max(limit, pinned.size))) break;
      }
      if (!rows.length) return;
      try {
        pending = Promise.resolve(store.write(rows));
        await pending;
        if (disposed) return;
        for (const { item, record, value, fingerprint } of rows) {
          if (protectedItem(item) || JSON.stringify(item) !== fingerprint) continue;
          // Preserve edits made while IndexedDB was committing the snapshot.
          for (const key of Object.keys(value)) {
            if (!indexFields.has(key)) delete item[key];
          }
          record.archived = true; resident.delete(record); archived++;
        }
      } catch (_) {
        // Private browsing, quota or disk errors must never destroy records.
        disabled = true;
      } finally {
        pending = null;
        schedule();
      }
    }
    async function ready(items) {
      const unique = [...new Set(items.filter(Boolean))];
      track(unique);
      unique.forEach(item => holds.set(item, (holds.get(item) || 0) + 1));
      try {
        // A record being written is still complete; holding it prevents eviction
        // after commit. Archived records are rehydrated in one local transaction.
        const loading = unique.filter(item => records.get(item).loading).map(item => records.get(item).loading);
        if (loading.length) await Promise.all(loading);
        const missing = unique.filter(item => records.get(item).archived);
        if (missing.length) {
          const read = store.read(missing.map(item => records.get(item).key)).then(values => {
            if (disposed) throw new Error('Record window closed');
            if (values.length !== missing.length || values.some(value => !value)) throw new Error('Local media record unavailable');
            missing.forEach((item, index) => {
              if (!values[index]) throw new Error('Local media record unavailable');
              // The live compact record wins for favorite/deletion/bookmark edits.
              const overlay = { ...item };
              Object.assign(item, values[index], overlay);
              const record = records.get(item);
              record.archived = false; resident.set(record, record.ref); archived--; reads++;
            });
          });
          missing.forEach(item => { records.get(item).loading = read; });
          try { await read; }
          finally { missing.forEach(item => { records.get(item).loading = null; }); }
          onChange();
        }
      } finally {
        unique.forEach(item => {
          const count = holds.get(item) - 1;
          if (count) holds.set(item, count); else holds.delete(item);
        });
        schedule();
      }
    }
    return {
      track,
      ready,
      full: item => !records.get(item)?.archived,
      pin(items, nextLimit = limit) { pinned = new Set(items.filter(Boolean)); limit = Math.max(32, nextLimit); schedule(); },
      stats: () => ({ resident: resident.size, archived, reads, limit, disabled }),
      async dispose() { disposed = true; win.clearTimeout(timer); resident.clear(); pinned.clear(); holds.clear(); await pending?.catch(() => {}); await store.dispose?.(); },
    };
  }
  const api = { create };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.EchoGalleryPhotoDataWindow = api;
})(typeof window !== 'undefined' ? window : globalThis);
