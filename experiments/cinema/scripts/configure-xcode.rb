# Configures the iOS and tvOS Xcode projects: bundle ids, device families and bundled fonts.
# Run with CocoaPods' Ruby: GEM_HOME=<cocoapods libexec> ruby scripts/configure-xcode.rb
require 'xcodeproj'
root = File.expand_path('..', __dir__)
{
  'ios' => {id: 'tv.getportico.experiment.cinema.ios', family: '1,2'},
  'tvos' => {id: 'tv.getportico.experiment.cinema.tv', family: '3'},
}.each do |dir, cfg|
  path = File.join(root, dir, 'PorticoCinema.xcodeproj')
  project = Xcodeproj::Project.open(path)
  target = project.targets.find { |t| t.name == 'PorticoCinema' }
  target.build_configurations.each do |bc|
    bc.build_settings['PRODUCT_BUNDLE_IDENTIFIER'] = cfg[:id]
    bc.build_settings['TARGETED_DEVICE_FAMILY'] = cfg[:family]
    bc.build_settings['MARKETING_VERSION'] = '0.1.0'
    bc.build_settings['CURRENT_PROJECT_VERSION'] = '1'
    bc.build_settings['DEVELOPMENT_TEAM'] = ''
    bc.build_settings['CODE_SIGN_STYLE'] = 'Automatic'
  end
  group = project.main_group['Fonts'] || project.main_group.new_group('Fonts', '../assets/fonts')
  present = Dir[File.join(root, 'assets/fonts/*.ttf')].map { |f| File.basename(f) }
  group.files.select { |f| !present.include?(f.path) }.each do |f|
    target.resources_build_phase.remove_file_reference(f)
    f.remove_from_project
  end
  Dir[File.join(root, 'assets/fonts/*.ttf')].sort.each do |font|
    name = File.basename(font)
    next if group.files.any? { |f| f.path == name }
    ref = group.new_reference(name)
    target.resources_build_phase.add_file_reference(ref, true)
  end
  project.save
  puts "configured #{dir}"
end
