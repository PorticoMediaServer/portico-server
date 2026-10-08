# Adds the Manrope fonts to both Xcode projects as bundle resources (idempotent).
require 'xcodeproj'
root = File.expand_path('..', __dir__)
fonts = Dir[File.join(root, 'assets/fonts/*.ttf')].sort
%w[ios tvos].each do |dir|
  proj = Xcodeproj::Project.open(File.join(root, dir, 'PorticoClassic.xcodeproj'))
  target = proj.targets.find { |t| t.name == 'PorticoClassic' }
  group = proj.main_group['Fonts'] || proj.main_group.new_group('Fonts')
  fonts.each do |f|
    rel = Pathname.new(f).relative_path_from(Pathname.new(File.join(root, dir))).to_s
    next if group.files.any? { |x| x.path == rel }
    ref = group.new_file(rel)
    target.resources_build_phase.add_file_reference(ref)
  end
  proj.save
end
puts 'ok'
