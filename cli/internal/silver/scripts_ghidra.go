package silver

const twdllSilverExportJava = `import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.AddressRange;
import ghidra.program.model.address.AddressSetView;
import ghidra.program.model.listing.Function;
import ghidra.program.model.listing.FunctionIterator;

import java.io.BufferedWriter;
import java.io.FileOutputStream;
import java.io.OutputStreamWriter;
import java.nio.charset.StandardCharsets;

public class TwdllSilverExport extends GhidraScript {
    private static String esc(String s) {
        if (s == null) return "";
        StringBuilder sb = new StringBuilder(s.length() + 16);
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': sb.append("\\\""); break;
                case '\\': sb.append("\\\\"); break;
                case '\b': sb.append("\\b"); break;
                case '\f': sb.append("\\f"); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                default:
                    if (c < 0x20) sb.append(String.format("\\u%04x", (int) c));
                    else sb.append(c);
            }
        }
        return sb.toString();
    }

    private static String hexEA(long v) {
        return "0x" + Long.toHexString(v);
    }

    @Override
    public void run() throws Exception {
        String outPath = System.getenv("TWDLL_SILVER_OUT");
        if (outPath == null || outPath.isEmpty()) {
            throw new IllegalArgumentException("TWDLL_SILVER_OUT is required");
        }
        if (currentProgram == null) {
            throw new IllegalStateException("currentProgram is null");
        }

        long t0 = System.currentTimeMillis();
        int count = 0;

        try (BufferedWriter bw = new BufferedWriter(
                new OutputStreamWriter(new FileOutputStream(outPath), StandardCharsets.UTF_8), 65536)) {
            FunctionIterator it = currentProgram.getFunctionManager().getFunctions(true);
            StringBuilder line = new StringBuilder(512);
            while (it.hasNext() && !monitor.isCancelled()) {
                Function fn = it.next();
                if (fn == null || fn.isExternal()) {
                    continue;
                }
                long ea = fn.getEntryPoint().getOffset();
                AddressSetView body = fn.getBody();
                long size = body != null ? body.getNumAddresses() : 0;
                String name = fn.getName(true);
                String cc = fn.getCallingConventionName();
                if (cc == null) cc = "";
                int purge = fn.isStackPurgeSizeValid() ? fn.getStackPurgeSize() : -1;
                boolean isThunk = fn.isThunk();
                boolean isNoRet = fn.hasNoReturn();
                long thunkTarget = 0;
                if (isThunk) {
                    Function tgt = fn.getThunkedFunction(false);
                    if (tgt != null && !tgt.isExternal()) {
                        thunkTarget = tgt.getEntryPoint().getOffset();
                    }
                }

                line.setLength(0);
                line.append("{\"ea\":\"").append(hexEA(ea)).append("\",");
                line.append("\"size\":").append(size).append(",");
                line.append("\"ranges\":[");
                if (body != null) {
                    boolean firstR = true;
                    for (AddressRange r : body.getAddressRanges()) {
                        if (!r.getMinAddress().isMemoryAddress()) continue;
                        long rStart = r.getMinAddress().getOffset();
                        long rEnd = rStart + r.getLength();
                        if (!firstR) line.append(",");
                        firstR = false;
                        line.append("[\"").append(hexEA(rStart)).append("\",\"").append(hexEA(rEnd)).append("\"]");
                    }
                }
                line.append("],");
                line.append("\"name\":\"").append(esc(name)).append("\",");
                line.append("\"calling_conv\":\"").append(esc(cc)).append("\",");
                line.append("\"stack_purge\":").append(purge).append(",");
                line.append("\"is_thunk\":").append(isThunk).append(",");
                line.append("\"is_noret\":").append(isNoRet).append(",");
                line.append("\"thunk_target_ea\":\"").append(hexEA(thunkTarget)).append("\"}");

                bw.write(line.toString());
                bw.newLine();
                count++;
            }
            bw.flush();
        }
        println(String.format("TwdllSilverExport: exported %d functions in %.2fs", count, (System.currentTimeMillis() - t0) / 1000.0));
    }
}
`

const twdllDecompExportJava = `import ghidra.app.decompiler.ClangLine;
import ghidra.app.decompiler.ClangToken;
import ghidra.app.decompiler.ClangTokenGroup;
import ghidra.app.decompiler.DecompInterface;
import ghidra.app.decompiler.DecompileOptions;
import ghidra.app.decompiler.DecompileResults;
import ghidra.app.decompiler.component.DecompilerUtils;
import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.listing.Function;
import ghidra.program.model.pcode.HighFunction;
import ghidra.program.model.pcode.HighSymbol;
import ghidra.util.task.TaskMonitor;

import java.io.BufferedWriter;
import java.io.File;
import java.io.FileOutputStream;
import java.io.OutputStreamWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.List;
import java.util.concurrent.ConcurrentLinkedQueue;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

public class TwdllDecompExport extends GhidraScript {
    private static class DecompJob {
        final String eaHex;
        final long ea;
        final long size;
        final String name;
        Function fn;

        DecompJob(String eaHex, long ea, long size, String name) {
            this.eaHex = eaHex;
            this.ea = ea;
            this.size = size;
            this.name = name;
        }
    }

    private static String esc(String s) {
        if (s == null) return "";
        StringBuilder sb = new StringBuilder(s.length() + 16);
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            switch (c) {
                case '"': sb.append("\\\""); break;
                case '\\': sb.append("\\\\"); break;
                case '\b': sb.append("\\b"); break;
                case '\f': sb.append("\\f"); break;
                case '\n': sb.append("\\n"); break;
                case '\r': sb.append("\\r"); break;
                case '\t': sb.append("\\t"); break;
                default:
                    if (c < 0x20) sb.append(String.format("\\u%04x", (int) c));
                    else sb.append(c);
            }
        }
        return sb.toString();
    }

    private static String hexEA(long v) {
        return "0x" + Long.toHexString(v);
    }

    private static int computeLineShift(String[] cLines, List<ClangLine> clangLines) {
        for (ClangLine cl : clangLines) {
            StringBuilder sb = new StringBuilder();
            for (ClangToken tok : cl.getAllTokens()) {
                sb.append(tok.toString());
            }
            String txt = sb.toString().trim();
            if (txt.isEmpty()) {
                continue;
            }
            int expectedIdx = cl.getLineNumber() - 1;
            if (expectedIdx >= 0 && expectedIdx < cLines.length && cLines[expectedIdx].trim().equals(txt)) {
                return 0;
            }
            int maxScan = Math.min(cLines.length, Math.max(0, expectedIdx) + 16);
            for (int k = 0; k < maxScan; k++) {
                if (cLines[k].trim().equals(txt)) {
                    return (k + 1) - cl.getLineNumber();
                }
            }
            break;
        }
        return 0;
    }

    @Override
    public void run() throws Exception {
        long t0 = System.currentTimeMillis();
        String inPath = System.getenv("TWDLL_DECOMP_IN");
        String outPath = System.getenv("TWDLL_DECOMP_OUT");
        int workers = Integer.parseInt(System.getenv().getOrDefault("TWDLL_DECOMP_WORKERS", "2"));
        int timeoutSecs = Integer.parseInt(System.getenv().getOrDefault("TWDLL_DECOMP_TIMEOUT_SECS", "15"));
        if (inPath == null || inPath.isEmpty() || outPath == null || outPath.isEmpty()) {
            throw new IllegalArgumentException("TWDLL_DECOMP_IN and TWDLL_DECOMP_OUT are required");
        }
        if (workers < 1) workers = 1;
        if (timeoutSecs < 0) timeoutSecs = 0;

        List<String> rawLines = Files.readAllLines(new File(inPath).toPath(), StandardCharsets.UTF_8);
        List<DecompJob> jobs = new ArrayList<>(rawLines.size());
        for (String ln : rawLines) {
            String trimmed = ln.trim();
            if (trimmed.isEmpty()) continue;
            String[] parts = trimmed.split("\t", 3);
            String eaHex = parts[0].trim();
            long eaVal = Long.parseUnsignedLong(eaHex.startsWith("0x") ? eaHex.substring(2) : eaHex, 16);
            long sz = parts.length > 1 ? Long.parseLong(parts[1].trim()) : 0;
            String nm = parts.length > 2 ? parts[2] : "";
            jobs.add(new DecompJob(hexEA(eaVal), eaVal, sz, nm));
        }

        // Pre-pass on the main thread: use getFunctionAt + createFunction only.
        // Never fall back to getFunctionContaining(addr), and never mutate FunctionManager from worker threads.
        int createdMissing = 0;
        for (DecompJob job : jobs) {
            if (monitor.isCancelled()) break;
            Address addr = toAddr(job.ea);
            Function fn = getFunctionAt(addr);
            if (fn == null) {
                try {
                    fn = createFunction(addr, null);
                    if (fn != null) {
                        createdMissing++;
                    }
                } catch (Exception ignored) {
                    fn = getFunctionAt(addr);
                }
            }
            job.fn = fn;
        }

        ConcurrentLinkedQueue<DecompJob> queue = new ConcurrentLinkedQueue<>(jobs);
        int actualWorkers = Math.min(workers, Math.max(1, jobs.size()));
        AtomicInteger completed = new AtomicInteger(0);
        AtomicInteger okCount = new AtomicInteger(0);
        AtomicInteger timeoutCount = new AtomicInteger(0);
        AtomicInteger failCount = new AtomicInteger(0);
        AtomicReference<Throwable> fatalErr = new AtomicReference<>(null);
        Object writeLock = new Object();

        try (BufferedWriter bw = new BufferedWriter(
                new OutputStreamWriter(new FileOutputStream(outPath), StandardCharsets.UTF_8), 65536)) {
            ExecutorService pool = Executors.newFixedThreadPool(actualWorkers);
            final int totalJobs = jobs.size();
            final int tSecs = timeoutSecs;

            for (int w = 0; w < actualWorkers; w++) {
                pool.submit(() -> {
                    DecompInterface ifc = new DecompInterface();
                    try {
                        DecompileOptions opts = new DecompileOptions();
                        ifc.setOptions(opts);
                        ifc.toggleCCode(true);
                        ifc.toggleSyntaxTree(true);
                        ifc.setSimplificationStyle("decompile");
                        if (!ifc.openProgram(currentProgram)) {
                            throw new RuntimeException("DecompInterface.openProgram failed");
                        }

                        StringBuilder rec = new StringBuilder(8192);
                        while (!monitor.isCancelled()) {
                            DecompJob job = queue.poll();
                            if (job == null) {
                                break;
                            }

                            rec.setLength(0);
                            if (job.fn == null) {
                                failCount.incrementAndGet();
                                rec.append("{\"event\":\"result\",\"ea\":\"").append(job.eaHex).append("\",");
                                rec.append("\"size\":").append(job.size).append(",");
                                rec.append("\"name\":\"").append(esc(job.name)).append("\",");
                                rec.append("\"status\":\"failed\",\"elapsed_ms\":0,\"error\":\"Function not found\"}");
                                synchronized (writeLock) {
                                    bw.write(rec.toString());
                                    bw.newLine();
                                    bw.flush();
                                }
                                completed.incrementAndGet();
                                continue;
                            }

                            long tf0 = System.currentTimeMillis();
                            DecompileResults res = ifc.decompileFunction(job.fn, tSecs, TaskMonitor.DUMMY);
                            long elMs = System.currentTimeMillis() - tf0;

                            if (res == null || !res.decompileCompleted() || res.getDecompiledFunction() == null) {
                                String errMsg = (res == null) ? "null results" : res.getErrorMessage();
                                if (errMsg == null || errMsg.isEmpty()) {
                                    errMsg = "timeout/incomplete";
                                }
                                String errLow = errMsg.toLowerCase();
                                boolean isTimeout = errLow.contains("timeout") || errLow.contains("timed out") || (tSecs > 0 && elMs >= (long) tSecs * 900L);
                                String status = isTimeout ? "timeout" : "failed";
                                if (isTimeout) {
                                    timeoutCount.incrementAndGet();
                                } else {
                                    failCount.incrementAndGet();
                                }

                                rec.append("{\"event\":\"result\",\"ea\":\"").append(job.eaHex).append("\",");
                                rec.append("\"size\":").append(job.size).append(",");
                                rec.append("\"name\":\"").append(esc(job.name)).append("\",");
                                rec.append("\"status\":\"").append(status).append("\",");
                                rec.append("\"elapsed_ms\":").append(elMs).append(",");
                                rec.append("\"error\":\"").append(esc(errMsg)).append("\"}");
                                synchronized (writeLock) {
                                    bw.write(rec.toString());
                                    bw.newLine();
                                    bw.flush();
                                }
                                completed.incrementAndGet();
                                continue;
                            }

                            okCount.incrementAndGet();
                            String rawC = res.getDecompiledFunction().getC().replace("\r\n", "\n").replace("\r", "\n");
                            String[] cLines = rawC.split("\n", -1);

                            rec.append("{\"event\":\"result\",\"ea\":\"").append(job.eaHex).append("\",");
                            rec.append("\"size\":").append(job.size).append(",");
                            rec.append("\"name\":\"").append(esc(job.name)).append("\",");
                            rec.append("\"status\":\"ok\",");
                            rec.append("\"elapsed_ms\":").append(elMs).append(",");
                            rec.append("\"raw_c\":\"").append(esc(rawC)).append("\",");
                            rec.append("\"line_ea\":[");

                            ClangTokenGroup markup = res.getCCodeMarkup();
                            if (markup != null) {
                                List<ClangLine> clangLines = DecompilerUtils.toLines(markup);
                                int shift = computeLineShift(cLines, clangLines);
                                boolean firstLine = true;
                                for (ClangLine cl : clangLines) {
                                    long minEa = 0;
                                    long maxEa = 0;
                                    boolean hasEa = false;
                                    for (ClangToken tok : cl.getAllTokens()) {
                                        Address aMin = tok.getMinAddress();
                                        if (aMin != null && aMin.isMemoryAddress()) {
                                            long v = aMin.getOffset();
                                            if (!hasEa || Long.compareUnsigned(v, minEa) < 0) minEa = v;
                                            if (!hasEa || Long.compareUnsigned(v, maxEa) > 0) maxEa = v;
                                            hasEa = true;
                                        }
                                        Address aMax = tok.getMaxAddress();
                                        if (aMax != null && aMax.isMemoryAddress()) {
                                            long v = aMax.getOffset();
                                            if (!hasEa || Long.compareUnsigned(v, minEa) < 0) minEa = v;
                                            if (!hasEa || Long.compareUnsigned(v, maxEa) > 0) maxEa = v;
                                            hasEa = true;
                                        }
                                    }
                                    int lineNum = cl.getLineNumber() + shift;
                                    if (hasEa && lineNum >= 1) {
                                        if (!firstLine) rec.append(",");
                                        firstLine = false;
                                        rec.append("[").append(lineNum)
                                           .append(",\"").append(hexEA(minEa))
                                           .append("\",\"").append(hexEA(maxEa)).append("\"]");
                                    }
                                }
                            }
                            rec.append("],\"vars\":[");

                            HighFunction hf = res.getHighFunction();
                            if (hf != null && hf.getLocalSymbolMap() != null) {
                                Iterator<HighSymbol> it = hf.getLocalSymbolMap().getSymbols();
                                boolean firstVar = true;
                                while (it.hasNext()) {
                                    HighSymbol sym = it.next();
                                    if (!firstVar) rec.append(",");
                                    firstVar = false;
                                    String dtName = sym.getDataType() != null ? sym.getDataType().getDisplayName() : "";
                                    String stor = sym.getStorage() != null ? sym.getStorage().toString() : "";
                                    rec.append("{\"name\":\"").append(esc(sym.getName())).append("\",");
                                    rec.append("\"type\":\"").append(esc(dtName)).append("\",");
                                    rec.append("\"is_arg\":").append(sym.isParameter()).append(",");
                                    rec.append("\"size\":").append(sym.getSize()).append(",");
                                    rec.append("\"storage\":\"").append(esc(stor)).append("\"}");
                                }
                            }
                            rec.append("]}");

                            synchronized (writeLock) {
                                bw.write(rec.toString());
                                bw.newLine();
                                bw.flush();
                            }
                            int done = completed.incrementAndGet();
                            if (done % 250 == 0 || done == totalJobs) {
                                println(String.format("TwdllDecompExport: %d/%d (ok=%d timeout=%d fail=%d)",
                                        done, totalJobs, okCount.get(), timeoutCount.get(), failCount.get()));
                            }
                        }
                    } catch (Throwable t) {
                        fatalErr.compareAndSet(null, t);
                    } finally {
                        ifc.dispose();
                    }
                });
            }

            pool.shutdown();
            pool.awaitTermination(365, TimeUnit.DAYS);
            bw.flush();
        }

        if (fatalErr.get() != null) {
            throw new RuntimeException("Ghidra decomp worker failed", fatalErr.get());
        }
        println(String.format("TwdllDecompExport: finished %d/%d (created=%d ok=%d timeout=%d fail=%d) in %.2fs",
                completed.get(), jobs.size(), createdMissing, okCount.get(), timeoutCount.get(), failCount.get(),
                (System.currentTimeMillis() - t0) / 1000.0));
    }
}
`
