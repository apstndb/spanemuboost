# Selection is numeric, not lexical or upload-time order. Latest may be LTS.
def omni_pattern: "^(?<year>[0-9]{4})[.]r(?<release>[0-9]+)(?<lts>-lts)?([.](?<patch>[0-9]+))?$";
def emulator_pattern: "^[0-9]+[.][0-9]+[.][0-9]+$";
def omni_version:
  capture(omni_pattern)
  | [(.year|tonumber), (.release|tonumber), ((.patch // "0")|tonumber), (.lts == null)];
def newest($tags; $kind; $lts):
  [$tags[] | select(test(if $kind == "omni" then omni_pattern else emulator_pattern end))
   | select(($lts|not) or contains("-lts"))]
  | if length == 0 then error("no matching release tags")
    else max_by([(if $kind == "omni" then omni_version else split(".")|map(tonumber) end), .]) end;
def select_tags($omni; $emulator):
  {DefaultOmniImage: newest($omni; "omni"; false),
   LatestLTSOmniImage: newest($omni; "omni"; true),
   DefaultEmulatorImage: newest($emulator; "emulator"; false)};

def image_evidence($repo; $tag; $digest; $manifest):
  if ($digest | test("^sha256:[0-9a-f]{64}$") | not) then error("missing or unsupported manifest digest")
  elif ($manifest|type) != "object" then error("expected a manifest object")
  elif ($manifest | has("manifests")) and
       (($manifest.manifests|type) != "array" or
        any($manifest.manifests[]; type != "object" or (has("platform") and (.platform|type) != "object")))
  then error("invalid manifest descriptors")
  else $manifest.mediaType as $type
  | ($type == "application/vnd.oci.image.index.v1+json" or
     $type == "application/vnd.docker.distribution.manifest.list.v2+json") as $index
  | if ($index or $type == "application/vnd.oci.image.manifest.v1+json" or
        $type == "application/vnd.docker.distribution.manifest.v2+json") | not
    then error("unsupported manifest media type")
    else {tag: $tag, image: ($repo + ":" + $tag),
          registry: ("https://" + ($repo|split("/")[0]) + "/v2/" + ($repo|split("/")[1:]|join("/")) + "/tags/list"),
          index_digest: (if $index then $digest else null end),
          manifest_digest: (if $index then null else $digest end),
          platforms: ([$manifest.manifests[]?.platform
            | select(.os != null and .os != "unknown" and .architecture != null and .architecture != "unknown")
            | [.os, .architecture, .variant] | map(select(. != null and . != "")) | join("/")] | unique),
          runtime_validation: "not_run"}
    end
  end;

def constant($source; $name):
  # Exactly the same literal-constant lookup as the original detector.
  [$source | scan("\\b" + $name + "\\s*=\\s*\"([^\"]+)\"") | .[0]]
  | if length == 0 then error("cannot find literal constant " + $name) else .[0] end;
def mentioned($notes; $tag):
  ($tag | gsub("[.]"; "\\.")) as $escaped
  | $notes | test("(?<![\\w.-])" + $escaped + "(?![\\w.-])");
def report($source; $notes; $notesUrl; $release; $omni; $lts; $emulator):
  if ($release|type) != "object" or $release.draft or $release.prerelease or
     (($release.tag_name // "" | ltrimstr("v") | test(emulator_pattern)) | not) or
     ($release.html_url|type) != "string"
  then error("unexpected GitHub latest emulator release")
  else {checked_at: (now|strftime("%Y-%m-%dT%H:%M:%S+00:00")),
    images: {
      DefaultOmniImage: ($omni + {release_notes: {url: $notesUrl, tag_mentioned: mentioned($notes; $omni.tag)}}),
      LatestLTSOmniImage: ($lts + {release_notes: {url: $notesUrl, tag_mentioned: mentioned($notes; $lts.tag)}}),
      DefaultEmulatorImage: ($emulator + {github_release: {url: $release.html_url, tag: $release.tag_name,
        matches_registry_latest: ($release.tag_name|ltrimstr("v")) == $emulator.tag}})
    }} | .images |= with_entries(.key as $name | .value += {current_image: constant($source; $name)}
       | .value.update_available = (.value.current_image != .value.image))
  end;
