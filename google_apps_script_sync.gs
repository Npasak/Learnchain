/**
 * ============================================================================
 * Google Apps Script สำหรับระบบ Auto-Sync & ฐานข้อมูลกลางบน Google Sheet
 * ระบบการเรียนรู้ทางการพยาบาล (Learn Chain Nurse)
 * งานการพยาบาลผู้ป่วยพิเศษ 2-3 ฝ่ายการพยาบาล
 * โรงพยาบาลศรีนครินทร์ คณะแพทยศาสตร์ มหาวิทยาลัยขอนแก่น
 * ============================================================================
 *
 * วิธีติดตั้งใช้งาน (ใช้เวลาเพียง 1 นาที):
 * 1. สร้างไฟล์ Google Sheet ใหม่ที่ https://sheets.new
 * 2. ไปที่เมนู ส่วนขยาย (Extensions) -> Apps Script
 * 3. ลบโค้ดเดิมออกทั้งหมด แล้ววางโค้ดไฟล์นี้ลงไป กดบันทึก (💾)
 * 4. กดปุ่ม "ทำให้ใช้งานได้" (Deploy) -> "การทำให้ใช้งานได้รายการใหม่" (New deployment)
 * 5. เลือกประเภทเป็น "เว็บแอป" (Web app):
 *    - ดำเนินการในฐานะ (Execute as): "ฉัน" (Me)
 *    - ผู้ที่มีสิทธิ์เข้าถึง (Who has access): "ทุกคน" (Anyone)
 * 6. กดปุ่ม Deploy แล้วคัดลอก URL (https://script.google.com/macros/s/.../exec)
 *    นำมาวางในช่อง "Google Apps Script URL" ในหน้าต่าง 🔄 ซิงค์เครื่อง (วิธีที่ 2)
 */

const NODE_TIMEOUT_MS = 3 * 60 * 1000; // ลบเครื่องที่ไม่ได้ใช้งานเกิน 3 นาที (180 วินาที) อัตโนมัติ

function doGet(e) {
  return handleRequest(e, null);
}

function doPost(e) {
  var body = null;
  if (e && e.postData && e.postData.contents) {
    try {
      body = JSON.parse(e.postData.contents);
    } catch (err) {}
  }
  return handleRequest(e, body);
}

function handleRequest(e, body) {
  var lock = LockService.getScriptLock();
  lock.tryLock(10000);
  try {
    var ss = SpreadsheetApp.getActiveSpreadsheet();
    var nowMs = new Date().getTime();
    var timeoutMs = (body && body.timeout_sec) ? (body.timeout_sec * 1000) : NODE_TIMEOUT_MS;

    // 1. อัปเดตรายชื่อเครื่องที่เชื่อมต่อ (ActiveNodes) และลบเครื่องที่ไม่ได้ใช้งานเกินเวลาออกอัตโนมัติ
    var activeNodes = updateAndPruneNodes(ss, body, nowMs, timeoutMs);

    // โหมด Signaling (action === "signal"): แลกเปลี่ยนเฉพาะ Peer, URL, IP, LocalIP, StabilityScore เพื่อลดภาระ GAS
    if (body && body.action === "signal") {
      return ContentService.createTextOutput(JSON.stringify({
        success: true,
        mode: "signal",
        server_time: new Date(nowMs).toISOString(),
        online_count: activeNodes.length,
        active_nodes: activeNodes
      })).setMimeType(ContentService.MimeType.JSON);
    }

    // 2. โหมด Fallback (action === "auto_sync"): หากมีข้อมูล Snapshot ส่งมาจากเครื่องลูก ให้บันทึกลง Google Sheet
    if (body && body.snapshot) {
      mergeSnapshotToSheets(ss, body.snapshot);
    }

    // 3. อ่านข้อมูลฐานข้อมูลทั้งหมดจาก Google Sheet ส่งกลับไปให้ทุกเครื่องซิงค์ตรงกัน
    var cloudSnapshot = readSnapshotFromSheets(ss);

    var responsePayload = {
      success: true,
      server_time: new Date(nowMs).toISOString(),
      online_count: activeNodes.length,
      active_nodes: activeNodes,
      snapshot: cloudSnapshot
    };

    return ContentService.createTextOutput(JSON.stringify(responsePayload))
      .setMimeType(ContentService.MimeType.JSON);
  } catch (err) {
    return ContentService.createTextOutput(JSON.stringify({
      success: false,
      error: String(err)
    })).setMimeType(ContentService.MimeType.JSON);
  } finally {
    lock.releaseLock();
  }
}

function getOrCreateSheet(ss, name, headers) {
  var sheet = ss.getSheetByName(name);
  if (!sheet) {
    sheet = ss.insertSheet(name);
    sheet.appendRow(headers);
    sheet.getRange(1, 1, 1, headers.length).setFontWeight("bold").setBackground("#e0f2fe");
  } else if (sheet.getLastRow() === 0) {
    sheet.appendRow(headers);
  }
  return sheet;
}

// จัดการตาราง ActiveNodes: บันทึก IP/URL เครื่องที่ออนไลน์ และลบเครื่องที่ไม่ได้ใช้งานเกินกำหนดเวลา
function updateAndPruneNodes(ss, body, nowMs, timeoutMs) {
  var sheet = getOrCreateSheet(ss, "ActiveNodes", [
    "NodeID", "Hostname", "LanURL", "LastSeenMs", "LastSeenText", "UsersCount", "LessonsCount", "CertsCount"
  ]);

  var rows = sheet.getDataRange().getValues();
  var nodeMap = {};

  for (var i = 1; i < rows.length; i++) {
    var r = rows[i];
    var nid = String(r[0] || "");
    var lastMs = Number(r[3] || 0);
    if (!nid) continue;
    // ลบออกหากไม่ได้ใช้งานเกินเวลาที่กำหนด
    if (nowMs - lastMs <= timeoutMs) {
      nodeMap[nid] = {
        node_id: nid,
        hostname: String(r[1] || ""),
        lan_url: String(r[2] || ""),
        last_seen_ms: lastMs,
        last_seen_text: String(r[4] || ""),
        users_count: Number(r[5] || 0),
        lessons_count: Number(r[6] || 0),
        certs_count: Number(r[7] || 0)
      };
    }
  }

  // หากเป็นการสั่งลบเครื่องเมื่อปิดโปรแกรม (action == "disconnect")
  if (body && body.action === "disconnect" && body.node_id) {
    delete nodeMap[String(body.node_id)];
  } else if (body && body.node_id) {
    var snap = body.snapshot || {};
    var nidKey = String(body.node_id);
    nodeMap[nidKey] = {
      node_id: nidKey,
      hostname: String(body.hostname || "Computer"),
      lan_url: String(body.lan_url || ""),
      lan_urls: Array.isArray(body.lan_urls) ? body.lan_urls : [],
      is_master: Boolean(body.is_master),
      stability_score: Number(body.stability_score || 0),
      last_seen_ms: nowMs,
      last_seen_text: Utilities.formatDate(new Date(nowMs), "GMT+7", "yyyy-MM-dd HH:mm:ss"),
      users_count: Number(body.users_count || (snap.users || []).length),
      lessons_count: Number(body.lessons_count || (snap.lessons || []).length),
      certs_count: Number(body.certs_count || (snap.certificates || []).length)
    };
  }

  // เขียนตาราง ActiveNodes ใหม่เฉพาะเครื่องที่ยังออนไลน์อยู่
  if (sheet.getLastRow() > 1) {
    sheet.getRange(2, 1, sheet.getLastRow() - 1, 8).clearContent();
  }
  var activeList = [];
  var writeRows = [];
  for (var k in nodeMap) {
    var n = nodeMap[k];
    activeList.push(n);
    writeRows.push([
      n.node_id, n.hostname, n.lan_url, n.last_seen_ms, n.last_seen_text,
      n.users_count, n.lessons_count, n.certs_count
    ]);
  }
  if (writeRows.length > 0) {
    sheet.getRange(2, 1, writeRows.length, 8).setValues(writeRows);
  }
  return activeList;
}

// ผสานข้อมูล Snapshot ลงใน Google Sheet (Users, Lessons, Assignments, Progress, Certificates)
function mergeSnapshotToSheets(ss, snap) {
  // 1. Users
  var uSheet = getOrCreateSheet(ss, "Users", ["WalletAddress", "Email", "Name", "Ward", "Role", "PasswordHash", "JSON"]);
  mergeByKey(uSheet, snap.users || [], function(u) {
    return (u.email ? String(u.email).toLowerCase() + "|" + (u.role || "learner") : String(u.wallet_address));
  }, function(u) {
    return [u.wallet_address || "", u.email || "", u.name || "", u.ward || "", u.role || "learner", u.password_hash || "", JSON.stringify(u)];
  });

  // 2. Lessons
  var lSheet = getOrCreateSheet(ss, "Lessons", ["Title", "SortOrder", "StudyHours", "DurationSec", "OpenPeriod", "YoutubeURL", "JSON"]);
  mergeByKey(lSheet, snap.lessons || [], function(l) {
    return String(l.title || "");
  }, function(l) {
    return [l.title || "", l.sort_order || 1, l.study_hours || 1, l.duration_sec || 60, l.open_period || "", l.youtube_url || "", JSON.stringify(l)];
  });

  // 3. Assignments
  var aSheet = getOrCreateSheet(ss, "Assignments", ["Key", "LessonTitle", "UserEmail", "UserWallet", "JSON"]);
  mergeByKey(aSheet, snap.assignments || [], function(a) {
    return String(a.lesson_title || "") + "|" + String(a.user_email || a.user_wallet || "").toLowerCase();
  }, function(a) {
    var key = String(a.lesson_title || "") + "|" + String(a.user_email || a.user_wallet || "").toLowerCase();
    return [key, a.lesson_title || "", a.user_email || "", a.user_wallet || "", JSON.stringify(a)];
  });

  // 4. Progress
  var pSheet = getOrCreateSheet(ss, "Progress", ["Key", "UserEmail", "LessonTitle", "WatchedSec", "Completed", "BestScore", "JSON"]);
  mergeProgressSheet(pSheet, snap.progress_list || []);

  // 5. Certificates
  var cSheet = getOrCreateSheet(ss, "Certificates", ["TokenID", "UserEmail", "LessonTitle", "StudyHours", "AverageScore", "TransactionHash", "JSON"]);
  mergeByKey(cSheet, snap.certificates || [], function(c) {
    return String(c.token_id || (c.user_email + "|" + c.lesson_title));
  }, function(c) {
    return [c.token_id || "", c.user_email || "", c.lesson_title || "", c.study_hours || 1, c.average_score || 0, c.transaction_hash || "", JSON.stringify(c)];
  });

  // 6. CoverFiles (แบ่ง chunk ไม่เกิน 40,000 ตัวอักษรต่อเซลล์เพื่อไม่เกินลิมิตของ Google Sheet)
  var cfSheet = getOrCreateSheet(ss, "CoverFiles", ["Filename", "ChunkIndex", "ChunkData"]);
  mergeCoverFilesSheet(cfSheet, snap.cover_files || []);
}

function mergeCoverFilesSheet(sheet, incomingFiles) {
  if (!incomingFiles || incomingFiles.length === 0) return;
  var CHUNK_SIZE = 40000;
  var data = sheet.getDataRange().getValues();
  var fileMap = {};
  for (var i = 1; i < data.length; i++) {
    var fname = String(data[i][0] || "");
    var idx = Number(data[i][1] || 0);
    var chunk = String(data[i][2] || "");
    if (!fname || !chunk) continue;
    if (!fileMap[fname]) fileMap[fname] = [];
    fileMap[fname][idx] = chunk;
  }
  var joinedMap = {};
  for (var f in fileMap) {
    joinedMap[f] = fileMap[f].join("");
  }
  for (var j = 0; j < incomingFiles.length; j++) {
    var cf = incomingFiles[j];
    if (cf && cf.filename && cf.data_base64) {
      joinedMap[cf.filename] = cf.data_base64;
    }
  }
  var outRows = [];
  for (var name in joinedMap) {
    var b64 = joinedMap[name];
    if (!b64) continue;
    var chunkIdx = 0;
    for (var pos = 0; pos < b64.length; pos += CHUNK_SIZE) {
      outRows.push([name, chunkIdx, b64.substring(pos, pos + CHUNK_SIZE)]);
      chunkIdx++;
    }
  }
  if (sheet.getLastRow() > 1) {
    sheet.getRange(2, 1, sheet.getLastRow() - 1, 3).clearContent();
  }
  if (outRows.length > 0) {
    sheet.getRange(2, 1, outRows.length, 3).setValues(outRows);
  }
}

function readCoverFilesSheet(ss) {
  var sheet = ss.getSheetByName("CoverFiles");
  if (!sheet || sheet.getLastRow() <= 1) return [];
  var data = sheet.getDataRange().getValues();
  var fileMap = {};
  for (var i = 1; i < data.length; i++) {
    var fname = String(data[i][0] || "");
    var idx = Number(data[i][1] || 0);
    var chunk = String(data[i][2] || "");
    if (!fname || !chunk) continue;
    if (!fileMap[fname]) fileMap[fname] = [];
    fileMap[fname][idx] = chunk;
  }
  var res = [];
  for (var name in fileMap) {
    res.push({
      filename: name,
      data_base64: fileMap[name].join("")
    });
  }
  return res;
}

function mergeByKey(sheet, incomingList, keyFn, rowBuilderFn) {
  if (!incomingList || incomingList.length === 0) return;
  var data = sheet.getDataRange().getValues();
  var ncols = data[0].length;
  var map = {};
  for (var i = 1; i < data.length; i++) {
    var jsonStr = data[i][ncols - 1];
    if (!jsonStr) continue;
    try {
      var obj = JSON.parse(jsonStr);
      map[keyFn(obj)] = obj;
    } catch (e) {}
  }
  for (var j = 0; j < incomingList.length; j++) {
    var item = incomingList[j];
    var k = keyFn(item);
    if (!k) continue;
    map[k] = item;
  }
  var outRows = [];
  for (var key in map) {
    outRows.push(rowBuilderFn(map[key]));
  }
  if (sheet.getLastRow() > 1) {
    sheet.getRange(2, 1, sheet.getLastRow() - 1, ncols).clearContent();
  }
  if (outRows.length > 0) {
    sheet.getRange(2, 1, outRows.length, ncols).setValues(outRows);
  }
}

function mergeProgressSheet(sheet, incomingList) {
  if (!incomingList || incomingList.length === 0) return;
  var data = sheet.getDataRange().getValues();
  var ncols = 7;
  var map = {};
  for (var i = 1; i < data.length; i++) {
    var jsonStr = data[i][6];
    if (!jsonStr) continue;
    try {
      var obj = JSON.parse(jsonStr);
      var k = String(obj.lesson_title || "") + "|" + String(obj.user_email || obj.user_wallet || "").toLowerCase();
      map[k] = obj;
    } catch (e) {}
  }
  for (var j = 0; j < incomingList.length; j++) {
    var sp = incomingList[j];
    var k2 = String(sp.lesson_title || "") + "|" + String(sp.user_email || sp.user_wallet || "").toLowerCase();
    if (!k2) continue;
    var ex = map[k2];
    if (!ex) {
      map[k2] = sp;
    } else {
      if ((sp.last_watched_sec || 0) > (ex.last_watched_sec || 0)) ex.last_watched_sec = sp.last_watched_sec;
      if (sp.video_watched) ex.video_watched = true;
      if (sp.pre_test_passed) ex.pre_test_passed = true;
      if (sp.post_test_passed) ex.post_test_passed = true;
      if (sp.is_completed) ex.is_completed = true;
      if ((sp.best_score || 0) > (ex.best_score || 0)) ex.best_score = sp.best_score;
      if ((sp.total_score || 0) > (ex.total_score || 0)) ex.total_score = sp.total_score;
      if (sp.on_chain_tx_hash && !ex.on_chain_tx_hash) ex.on_chain_tx_hash = sp.on_chain_tx_hash;
      map[k2] = ex;
    }
  }
  var outRows = [];
  for (var key in map) {
    var p = map[key];
    outRows.push([key, p.user_email || "", p.lesson_title || "", p.last_watched_sec || 0, p.is_completed ? "สำเร็จ" : "กำลังเรียน", p.best_score || 0, JSON.stringify(p)]);
  }
  if (sheet.getLastRow() > 1) {
    sheet.getRange(2, 1, sheet.getLastRow() - 1, ncols).clearContent();
  }
  if (outRows.length > 0) {
    sheet.getRange(2, 1, outRows.length, ncols).setValues(outRows);
  }
}

function readJsonColumn(ss, sheetName) {
  var sheet = ss.getSheetByName(sheetName);
  if (!sheet || sheet.getLastRow() <= 1) return [];
  var data = sheet.getDataRange().getValues();
  var ncols = data[0].length;
  var res = [];
  for (var i = 1; i < data.length; i++) {
    var jsonStr = data[i][ncols - 1];
    if (!jsonStr) continue;
    try {
      res.push(JSON.parse(jsonStr));
    } catch (e) {}
  }
  return res;
}

function readSnapshotFromSheets(ss) {
  return {
    version: "1.0",
    exported_at: new Date().toISOString(),
    source_host: "GoogleSheetCloudDB",
    users: readJsonColumn(ss, "Users"),
    lessons: readJsonColumn(ss, "Lessons"),
    assignments: readJsonColumn(ss, "Assignments"),
    progress_list: readJsonColumn(ss, "Progress"),
    certificates: readJsonColumn(ss, "Certificates"),
    cover_files: readCoverFilesSheet(ss)
  };
}
