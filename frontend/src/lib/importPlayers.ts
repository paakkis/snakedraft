// Reads Steam profiles out of a CSV or Excel (.xlsx) file.
//
// The file layout does not matter: every Steam profile URL or Steam64 ID
// found anywhere in the file is picked up, whatever column it is in.

export interface ImportResult {
  inputs: string[]     // profile URLs / Steam64 IDs, in file order, no duplicates
  warnings: string[]
}

const PROFILE_URL = /steamcommunity\.com\/(id|profiles)\/([^\s"'<>,;|/?#]+)/gi
const STEAM64 = /(?<!\d)7656119\d{10}(?!\d)/g
// Excel stores a 17-digit ID typed into a number cell as 7.65611980314673E+16
const ROUNDED_STEAM64 = /7\.656119\d*E\+?16/i

/** Finds every Steam profile URL and Steam64 ID in a piece of text. */
export function extractSteamInputs(text: string): ImportResult {
  const found: { index: number; key: string; input: string }[] = []
  for (const m of text.matchAll(PROFILE_URL)) {
    const kind = m[1].toLowerCase()
    const name = decodeURIComponent(m[2])
    found.push(kind === 'profiles'
      ? { index: m.index, key: name, input: name }
      : { index: m.index, key: `id/${name.toLowerCase()}`, input: `https://steamcommunity.com/id/${name}` })
  }
  for (const m of text.matchAll(STEAM64)) {
    found.push({ index: m.index, key: m[0], input: m[0] })
  }
  found.sort((a, b) => a.index - b.index)

  const seen = new Set<string>()
  const inputs: string[] = []
  for (const f of found) {
    if (!seen.has(f.key)) {
      seen.add(f.key)
      inputs.push(f.input)
    }
  }

  const warnings: string[] = []
  if (ROUNDED_STEAM64.test(text)) {
    warnings.push('Excel rounded some numeric Steam IDs. Format the column as text or use profile URLs.')
  }
  return { inputs, warnings }
}

/** Reads a .csv, .txt or .xlsx file and returns the Steam profiles in it. */
export async function readPlayerFile(file: File): Promise<ImportResult> {
  const name = file.name.toLowerCase()
  if (name.endsWith('.xls')) {
    throw new Error('.xls is not supported. Save as .xlsx or .csv.')
  }
  const bytes = new Uint8Array(await file.arrayBuffer())
  const isZip = bytes[0] === 0x50 && bytes[1] === 0x4b // "PK": xlsx is a zip of XML files
  const text = isZip ? await xlsxText(bytes) : new TextDecoder().decode(bytes)
  return extractSteamInputs(text)
}

// --- minimal .xlsx reader ----------------------------------------------------
// Cell text lives in xl/sharedStrings.xml, numbers in the sheet XML and
// hyperlinks in the .rels files, so all XML in the archive is searched.

async function xlsxText(zip: Uint8Array): Promise<string> {
  const view = new DataView(zip.buffer, zip.byteOffset, zip.byteLength)
  const decoder = new TextDecoder()

  // End-of-central-directory record: scan backwards for its signature
  let eocd = zip.length - 22
  while (eocd >= 0 && view.getUint32(eocd, true) !== 0x06054b50) eocd--
  if (eocd < 0) throw new Error('Not a valid .xlsx file.')

  const entries = view.getUint16(eocd + 10, true)
  let pos = view.getUint32(eocd + 16, true)
  const parts: string[] = []

  for (let i = 0; i < entries && view.getUint32(pos, true) === 0x02014b50; i++) {
    const method = view.getUint16(pos + 10, true)
    const compressedSize = view.getUint32(pos + 20, true)
    const nameLength = view.getUint16(pos + 28, true)
    const extraLength = view.getUint16(pos + 30, true)
    const commentLength = view.getUint16(pos + 32, true)
    const localHeader = view.getUint32(pos + 42, true)
    const entryName = decoder.decode(zip.subarray(pos + 46, pos + 46 + nameLength))
    pos += 46 + nameLength + extraLength + commentLength

    if (!/\.(xml|rels)$/i.test(entryName)) continue
    const dataStart = localHeader + 30
      + view.getUint16(localHeader + 26, true) + view.getUint16(localHeader + 28, true)
    const data = zip.slice(dataStart, dataStart + compressedSize)
    if (method === 0) {
      parts.push(decoder.decode(data))
    } else if (method === 8) {
      const stream = new Blob([data]).stream().pipeThrough(new DecompressionStream('deflate-raw'))
      parts.push(await new Response(stream).text())
    }
  }
  return parts.join('\n').replace(/&amp;/g, '&')
}
