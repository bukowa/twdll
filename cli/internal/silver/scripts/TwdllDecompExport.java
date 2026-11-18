import com.google.gson.Gson;
import com.google.gson.GsonBuilder;
import generic.concurrent.GThreadPool;
import ghidra.app.decompiler.ClangLine;
import ghidra.app.decompiler.ClangToken;
import ghidra.app.decompiler.ClangTokenGroup;
import ghidra.app.decompiler.DecompInterface;
import ghidra.app.decompiler.DecompileOptions;
import ghidra.app.decompiler.DecompileResults;
import ghidra.app.decompiler.PrettyPrinter;
import ghidra.app.decompiler.parallel.DecompileConfigurer;
import ghidra.app.decompiler.parallel.DecompilerCallback;
import ghidra.app.decompiler.parallel.ParallelDecompiler;
import ghidra.app.script.GhidraScript;
import ghidra.program.model.address.Address;
import ghidra.program.model.listing.Function;
import ghidra.program.model.pcode.HighFunction;
import ghidra.program.model.pcode.HighSymbol;
import ghidra.program.model.symbol.IllegalCharCppTransformer;
import ghidra.util.task.TaskMonitor;

import java.io.BufferedWriter;
import java.io.File;
import java.io.FileOutputStream;
import java.io.OutputStreamWriter;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.Iterator;
import java.util.List;
import java.util.Map;
import java.util.concurrent.atomic.AtomicInteger;

public class TwdllDecompExport extends GhidraScript {
    private static final Gson GSON = new GsonBuilder().disableHtmlEscaping().create();

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

    @SuppressWarnings("unused")
    private static class DecompVarRec {
        final String name;
        final String type;
        final boolean is_arg;
        final int size;
        final String storage;

        DecompVarRec(String name, String type, boolean isArg, int size, String storage) {
            this.name = name;
            this.type = type;
            this.is_arg = isArg;
            this.size = size;
            this.storage = storage;
        }
    }

    @SuppressWarnings("unused")
    private static class DecompFrameRec {
        final String event = "result";
        final String ea;
        final long size;
        final String name;
        final String status;
        final long elapsed_ms;
        final String raw_c;
        final List<Object[]> line_ea;
        final List<DecompVarRec> vars;
        final String error;

        DecompFrameRec(
                String ea,
                long size,
                String name,
                String status,
                long elapsedMs,
                String rawC,
                List<Object[]> lineEa,
                List<DecompVarRec> vars,
                String error
        ) {
            this.ea = ea;
            this.size = size;
            this.name = name;
            this.status = status;
            this.elapsed_ms = elapsedMs;
            this.raw_c = rawC;
            this.line_ea = lineEa;
            this.vars = vars;
            this.error = error;
        }
    }

    @SuppressWarnings("unused")
    private static class DecompPhaseRec {
        final String event = "phase";
        final String phase;

        DecompPhaseRec(String phase) {
            this.phase = phase;
        }
    }

    private static void emitPhase(BufferedWriter bw, Object writeLock, String phase) throws Exception {
        String line = GSON.toJson(new DecompPhaseRec(phase));
        synchronized (writeLock) {
            bw.write(line);
            bw.newLine();
            bw.flush();
        }
    }

    private static String hexEA(long v) {
        return "0x" + Long.toHexString(v);
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

        final int totalJobs = jobs.size();
        List<Function> validFuncs = new ArrayList<>(totalJobs);
        Map<Long, DecompJob> mutableJobByEA = new HashMap<>(totalJobs * 2);
        List<DecompJob> missingJobs = new ArrayList<>();

        AtomicInteger completed = new AtomicInteger(0);
        AtomicInteger okCount = new AtomicInteger(0);
        AtomicInteger timeoutCount = new AtomicInteger(0);
        AtomicInteger failCount = new AtomicInteger(0);
        Object writeLock = new Object();
        final long[] lastFlushMs = new long[] { System.currentTimeMillis() };

        try (BufferedWriter bw = new BufferedWriter(
                new OutputStreamWriter(new FileOutputStream(outPath), StandardCharsets.UTF_8), 65536)) {
            emitPhase(bw, writeLock, "loading funcs");
            for (DecompJob job : jobs) {
                if (monitor.isCancelled()) break;
                job.fn = getFunctionAt(toAddr(job.ea));
                if (job.fn == null) {
                    missingJobs.add(job);
                } else {
                    validFuncs.add(job.fn);
                    mutableJobByEA.put(job.fn.getEntryPoint().getOffset(), job);
                }
            }
            final Map<Long, DecompJob> jobByEA = Collections.unmodifiableMap(mutableJobByEA);
            int actualWorkers = Math.min(workers, Math.max(1, validFuncs.size()));

            for (DecompJob missing : missingJobs) {
                failCount.incrementAndGet();
                completed.incrementAndGet();
                bw.write(GSON.toJson(new DecompFrameRec(
                        missing.eaHex,
                        missing.size,
                        missing.name,
                        "failed",
                        0,
                        null,
                        null,
                        null,
                        "Function not found"
                )));
                bw.newLine();
                bw.flush();
            }

            if (!validFuncs.isEmpty() && !monitor.isCancelled()) {
                // Pre-warm DataTypeManagerDB on a single thread so parallel DecompInterface
                // workers do not contend on cold 600k+ DWARF type loads during their timeout window.
                emitPhase(bw, writeLock, "loading types");
                currentProgram.getDataTypeManager().getDataTypeCount(true);
                emitPhase(bw, writeLock, "decomp");

                GThreadPool pool = GThreadPool.getSharedThreadPool("Parallel Decompiler");
                pool.setMaxThreadCount(actualWorkers);

                DecompileConfigurer configurer = ifc -> {
                    DecompileOptions opts = new DecompileOptions();
                    opts.grabFromProgram(currentProgram);
                    opts.setMaxPayloadMBytes(256);
                    ifc.setOptions(opts);
                    ifc.toggleCCode(true);
                    ifc.toggleSyntaxTree(true);
                    ifc.setSimplificationStyle("decompile");
                };

                final ThreadLocal<Long> fnStartMs = new ThreadLocal<>();
                DecompilerCallback<Void> callback = new DecompilerCallback<Void>(currentProgram, configurer) {
                    @Override
                    public Void process(Function f, TaskMonitor mon) throws Exception {
                        fnStartMs.set(System.currentTimeMillis());
                        return super.process(f, mon);
                    }

                    @Override
                    public Void process(DecompileResults res, TaskMonitor mon) throws Exception {
                        Long startedAt = fnStartMs.get();
                        long elMs = startedAt != null ? Math.max(0L, System.currentTimeMillis() - startedAt) : 0L;

                        Function fn = res != null ? res.getFunction() : null;
                        if (fn == null) {
                            return null;
                        }
                        long entryEA = fn.getEntryPoint().getOffset();
                        DecompJob job = jobByEA.get(entryEA);
                        String eaHex = job != null ? job.eaHex : hexEA(entryEA);
                        long jobSize = job != null ? job.size : (fn.getBody() != null ? fn.getBody().getNumAddresses() : 0);
                        String jobName = job != null ? job.name : (fn.getName(true) != null ? fn.getName(true) : "");

                        if (!res.decompileCompleted() || res.getCCodeMarkup() == null) {
                            String errMsg = res.getErrorMessage();
                            if (errMsg == null || errMsg.isBlank()) {
                                errMsg = res.isTimedOut() ? "Decompiler timed out" : "Decompilation incomplete";
                            } else {
                                errMsg = errMsg.trim();
                            }
                            String errLow = errMsg.toLowerCase();
                            boolean isTimeout = res.isTimedOut() || errLow.contains("timeout") || errLow.contains("timed out");
                            String status = isTimeout ? "timeout" : "failed";
                            if (isTimeout) {
                                timeoutCount.incrementAndGet();
                            } else {
                                failCount.incrementAndGet();
                            }

                            int done = completed.incrementAndGet();
                            String jsonLine = GSON.toJson(new DecompFrameRec(
                                    eaHex,
                                    jobSize,
                                    jobName,
                                    status,
                                    elMs,
                                    null,
                                    null,
                                    null,
                                    errMsg
                            ));
                            synchronized (writeLock) {
                                bw.write(jsonLine);
                                bw.newLine();
                                bw.flush();
                            }
                            if (done % 250 == 0 || done == totalJobs) {
                                println(String.format("TwdllDecompExport: %d/%d (ok=%d timeout=%d fail=%d)",
                                        done, totalJobs, okCount.get(), timeoutCount.get(), failCount.get()));
                            }
                            return null;
                        }

                        ClangTokenGroup markup = res.getCCodeMarkup();
                        PrettyPrinter printer = new PrettyPrinter(fn, markup, new IllegalCharCppTransformer());
                        String rawC = printer.print().getC().replace("\r\n", "\n").replace("\r", "\n");

                        List<Object[]> lineEa = new ArrayList<>();
                        for (ClangLine cl : printer.getLines()) {
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
                            int lineNum = cl.getLineNumber();
                            if (hasEa && lineNum >= 1) {
                                lineEa.add(new Object[]{lineNum, hexEA(minEa), hexEA(maxEa)});
                            }
                        }

                        List<DecompVarRec> vars = new ArrayList<>();
                        HighFunction hf = res.getHighFunction();
                        if (hf != null && hf.getLocalSymbolMap() != null) {
                            Iterator<HighSymbol> it = hf.getLocalSymbolMap().getSymbols();
                            while (it.hasNext()) {
                                HighSymbol sym = it.next();
                                String symName = sym.getName() != null ? sym.getName() : "";
                                String dtName = sym.getDataType() != null ? sym.getDataType().getDisplayName() : "";
                                String stor = sym.getStorage() != null ? sym.getStorage().toString() : "";
                                vars.add(new DecompVarRec(symName, dtName, sym.isParameter(), sym.getSize(), stor));
                            }
                        }

                        okCount.incrementAndGet();
                        int done = completed.incrementAndGet();
                        String jsonLine = GSON.toJson(new DecompFrameRec(
                                eaHex,
                                jobSize,
                                jobName,
                                "ok",
                                elMs,
                                rawC,
                                lineEa,
                                vars,
                                null
                        ));
                        synchronized (writeLock) {
                            bw.write(jsonLine);
                            bw.newLine();
                            long nowMs = System.currentTimeMillis();
                            if (done <= 32 || done % 32 == 0 || done == totalJobs || (nowMs - lastFlushMs[0]) >= 500) {
                                bw.flush();
                                lastFlushMs[0] = nowMs;
                            }
                        }
                        if (done % 250 == 0 || done == totalJobs) {
                            println(String.format("TwdllDecompExport: %d/%d (ok=%d timeout=%d fail=%d)",
                                    done, totalJobs, okCount.get(), timeoutCount.get(), failCount.get()));
                        }
                        return null;
                    }
                };
                callback.setTimeout(timeoutSecs);
                try {
                    ParallelDecompiler.decompileFunctions(callback, currentProgram, validFuncs.iterator(), r -> {}, monitor);
                } finally {
                    callback.dispose();
                }
            }
            bw.flush();
        }

        println(String.format("TwdllDecompExport: finished %d/%d (ok=%d timeout=%d fail=%d) in %.2fs",
                completed.get(), totalJobs, okCount.get(), timeoutCount.get(), failCount.get(),
                (System.currentTimeMillis() - t0) / 1000.0));
    }
}
